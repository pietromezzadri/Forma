package io

// Reading a 3MF's parts and separating them into files of their own, for
// printing: what the build puts on the plate (objects, objects made of
// components, each item's placement), each part named as the file names it
// (its name, or a slicer's own: PrusaSlicer keeps them in
// Metadata/Slic3r_PE_model.config), optionally broken into its loose
// pieces. The model can be a gigabyte of XML (a slicer project of a
// detailed figure), so it is scanned tag by tag, not decoded, and meshes
// are kept in float32.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	stdio "io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// MeshPart is a triangle mesh: XYZ triples and index triples.
type MeshPart struct {
	Name string
	Pos  []float32
	Tris []uint32
	// its colour, when the file gives one (core base materials)
	Color    [3]float64
	HasColor bool
}

// Triangles counts its triangles.
func (p MeshPart) Triangles() int { return len(p.Tris) / 3 }

// xform3 is a 3MF transform: x' = x·m0 + y·m3 + z·m6 + m9 and so on (its
// twelve numbers, rows of a 4×3 matrix).
type xform3 [12]float64

var ident3 = xform3{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}

func parseXform(s string) (xform3, error) {
	f := strings.Fields(s)
	if len(f) != 12 {
		return ident3, fmt.Errorf("a transform of %d numbers", len(f))
	}
	var m xform3
	for i, v := range f {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return ident3, err
		}
		m[i] = x
	}
	return m, nil
}

// then is a followed by b.
func (a xform3) then(b xform3) xform3 {
	var m xform3
	for r := 0; r < 4; r++ {
		for c := 0; c < 3; c++ {
			v := a[r*3]*b[c] + a[r*3+1]*b[3+c] + a[r*3+2]*b[6+c]
			if r == 3 {
				v += b[9+c]
			}
			m[r*3+c] = v
		}
	}
	return m
}

func (m xform3) apply(x, y, z float64) (float64, float64, float64) {
	return x*m[0] + y*m[3] + z*m[6] + m[9], x*m[1] + y*m[4] + z*m[7] + m[10], x*m[2] + y*m[5] + z*m[8] + m[11]
}

type mfObject struct {
	name  string
	pos   []float32
	tris  []uint32
	comps []mfComp
	pid   int // its base materials group and index (pid 0: none)
	pidx  int
}

type mfComp struct {
	id int
	x  xform3
}

// Read3MFParts reads the parts a 3MF's build puts on the plate, each
// placed as the build places it (an object of components gives a part
// for each).
func Read3MFParts(path string) ([]MeshPart, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("not a 3MF (zip) file: %v", err)
	}
	defer zr.Close()
	return read3MF(&zr.Reader)
}

// Read3MFPartsFrom reads the parts of a 3MF held in memory.
func Read3MFPartsFrom(data []byte) ([]MeshPart, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a 3MF (zip) file: %v", err)
	}
	return read3MF(zr)
}

func read3MF(zr *zip.Reader) ([]MeshPart, error) {
	var model *zip.File
	names := map[int]string{}
	for _, f := range zr.File {
		switch {
		case strings.EqualFold(f.Name, "3D/3dmodel.model") || (model == nil && strings.HasSuffix(strings.ToLower(f.Name), ".model")):
			model = f
		case f.Name == "Metadata/Slic3r_PE_model.config":
			slicerNames(f, names)
		}
	}
	if model == nil {
		return nil, fmt.Errorf("no 3D model in the file")
	}
	rc, err := model.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	objs, build, bases, err := scanModel(bufio.NewReaderSize(rc, 1<<20))
	if err != nil {
		return nil, err
	}
	for id, o := range objs {
		if o.name == "" {
			o.name = names[id]
		}
	}
	// what the build places: every object holding a mesh is a part of its
	// own, an object of components giving a part for each (a design's
	// parts kept together on the plate are still parts to print apart)
	var out []MeshPart
	var walk func(id int, x xform3, name string, depth int) error
	walk = func(id int, x xform3, name string, depth int) error {
		o := objs[id]
		if o == nil {
			return fmt.Errorf("the file names object %d, which isn't there", id)
		}
		if depth > 32 {
			return fmt.Errorf("components nested too deep")
		}
		if o.name != "" {
			name = o.name
		}
		if len(o.tris) > 0 {
			p := MeshPart{Name: name, Pos: make([]float32, 0, len(o.pos)), Tris: append([]uint32(nil), o.tris...)}
			if cs := bases[o.pid]; o.pid != 0 && o.pidx >= 0 && o.pidx < len(cs) {
				p.Color, p.HasColor = cs[o.pidx], true
			}
			for i := 0; i+2 < len(o.pos); i += 3 {
				a, b, c := x.apply(float64(o.pos[i]), float64(o.pos[i+1]), float64(o.pos[i+2]))
				p.Pos = append(p.Pos, float32(a), float32(b), float32(c))
			}
			out = append(out, p)
		}
		for k, c := range o.comps {
			sub := name
			if len(o.comps) > 1 {
				sub = fmt.Sprintf("%s %d", name, k+1)
			}
			if err := walk(c.id, c.x.then(x), sub, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if len(build) == 0 { // no build: every object that isn't a component
		used := map[int]bool{}
		for _, o := range objs {
			for _, c := range o.comps {
				used[c.id] = true
			}
		}
		var ids []int
		for id := range objs {
			if !used[id] {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		for _, id := range ids {
			build = append(build, mfComp{id, ident3})
		}
	}
	for k, it := range build {
		if err := walk(it.id, it.x, fmt.Sprintf("part %d", k+1), 0); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the file has no triangles")
	}
	return out, nil
}

// slicerNames reads PrusaSlicer's names of the objects (by id).
func slicerNames(f *zip.File, names map[int]string) {
	rc, err := f.Open()
	if err != nil {
		return
	}
	defer rc.Close()
	var cfg struct {
		Objects []struct {
			ID   int `xml:"id,attr"`
			Meta []struct {
				Type  string `xml:"type,attr"`
				Key   string `xml:"key,attr"`
				Value string `xml:"value,attr"`
			} `xml:"metadata"`
		} `xml:"object"`
	}
	if xml.NewDecoder(rc).Decode(&cfg) != nil {
		return
	}
	for _, o := range cfg.Objects {
		for _, m := range o.Meta {
			if m.Type == "object" && m.Key == "name" {
				names[o.ID] = m.Value
			}
		}
	}
}

// scanModel reads the model's objects and build items tag by tag.
func scanModel(r *bufio.Reader) (map[int]*mfObject, []mfComp, map[int][][3]float64, error) {
	bases := map[int][][3]float64{} // base materials groups' colours
	group := 0
	objs := map[int]*mfObject{}
	var build []mfComp
	var cur *mfObject
	var tag []byte
	for {
		// to the next tag
		if _, err := r.ReadSlice('<'); err != nil {
			if err == stdio.EOF {
				break
			}
			if err != bufio.ErrBufferFull {
				return nil, nil, nil, err
			}
			continue
		}
		tag = tag[:0]
		for {
			b, err := r.ReadSlice('>')
			tag = append(tag, b...)
			if err == nil {
				break
			}
			if err != bufio.ErrBufferFull {
				return nil, nil, nil, fmt.Errorf("the model ends inside a tag")
			}
		}
		tag = tag[:len(tag)-1]
		if len(tag) == 0 || tag[0] == '?' || tag[0] == '!' {
			continue
		}
		if tag[0] == '/' {
			if bytes.Equal(localName(tag[1:]), []byte("object")) {
				cur = nil
			}
			continue
		}
		name, attrs := splitTag(tag)
		switch string(name) {
		case "vertex":
			if cur == nil {
				continue
			}
			var v [3]float32
			for i, k := range []string{"x", "y", "z"} {
				x, err := strconv.ParseFloat(attr(attrs, k), 32)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("a vertex with a bad %s: %v", k, err)
				}
				v[i] = float32(x)
			}
			cur.pos = append(cur.pos, v[0], v[1], v[2])
		case "triangle":
			if cur == nil {
				continue
			}
			for _, k := range []string{"v1", "v2", "v3"} {
				x, err := strconv.ParseUint(attr(attrs, k), 10, 32)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("a triangle with a bad %s: %v", k, err)
				}
				cur.tris = append(cur.tris, uint32(x))
			}
		case "object":
			id, err := strconv.Atoi(attr(attrs, "id"))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("an object with a bad id")
			}
			cur = &mfObject{name: attr(attrs, "name")}
			if pid, err := strconv.Atoi(attr(attrs, "pid")); err == nil {
				cur.pid = pid
				cur.pidx, _ = strconv.Atoi(attr(attrs, "pindex"))
			}
			objs[id] = cur
		case "basematerials":
			group, _ = strconv.Atoi(attr(attrs, "id"))
		case "base":
			if c, ok := parseDisplayColor(attr(attrs, "displaycolor")); ok {
				bases[group] = append(bases[group], c)
			} else {
				bases[group] = append(bases[group], [3]float64{0.7, 0.7, 0.7})
			}
		case "component", "item":
			id, err := strconv.Atoi(attr(attrs, "objectid"))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("a %s with a bad objectid", name)
			}
			x := ident3
			if t := attr(attrs, "transform"); t != "" {
				if x, err = parseXform(t); err != nil {
					return nil, nil, nil, err
				}
			}
			if string(name) == "item" {
				build = append(build, mfComp{id, x})
			} else if cur != nil {
				cur.comps = append(cur.comps, mfComp{id, x})
			}
		}
	}
	for id, o := range objs {
		n := uint32(len(o.pos) / 3)
		for _, t := range o.tris {
			if t >= n {
				return nil, nil, nil, fmt.Errorf("object %d: a triangle names vertex %d of %d", id, t, n)
			}
		}
	}
	return objs, build, bases, nil
}

// localName drops a namespace prefix ("p:object" → "object").
func localName(b []byte) []byte {
	if i := bytes.IndexByte(b, ':'); i >= 0 {
		return b[i+1:]
	}
	return b
}

// splitTag is a tag's local name and the text of its attributes.
func splitTag(tag []byte) ([]byte, []byte) {
	tag = bytes.TrimSuffix(tag, []byte("/"))
	i := bytes.IndexFunc(tag, unicode.IsSpace)
	if i < 0 {
		return localName(tag), nil
	}
	return localName(tag[:i]), tag[i:]
}

// attr finds attribute k's value (unescaped) in a tag's attribute text.
func attr(attrs []byte, k string) string {
	for i := 0; i < len(attrs); {
		j := bytes.IndexByte(attrs[i:], '=')
		if j < 0 {
			return ""
		}
		key := bytes.TrimSpace(attrs[i : i+j])
		i += j + 1
		for i < len(attrs) && attrs[i] != '"' && attrs[i] != '\'' {
			i++
		}
		if i >= len(attrs) {
			return ""
		}
		q := attrs[i]
		e := bytes.IndexByte(attrs[i+1:], q)
		if e < 0 {
			return ""
		}
		val := attrs[i+1 : i+1+e]
		i += e + 2
		if string(localName(key)) == k {
			if bytes.IndexByte(val, '&') >= 0 {
				return unescape(string(val))
			}
			return string(val)
		}
	}
	return ""
}

func unescape(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&")
	return r.Replace(s)
}

// LoosePieces breaks a part into its pieces that share no vertex (vertices
// at the same place counted as one), most triangles first, named "<name> 1", "<name>
// 2"…; a part in one piece comes back as it is.
func LoosePieces(p MeshPart) []MeshPart {
	n := len(p.Pos) / 3
	// vertices at the same place are one
	same := make([]int32, n)
	seen := make(map[[3]float32]int32, n)
	for i := 0; i < n; i++ {
		k := [3]float32{p.Pos[3*i], p.Pos[3*i+1], p.Pos[3*i+2]}
		if j, ok := seen[k]; ok {
			same[i] = j
		} else {
			seen[k] = int32(i)
			same[i] = int32(i)
		}
	}
	parent := make([]int32, n)
	for i := range parent {
		parent[i] = int32(i)
	}
	var find func(int32) int32
	find = func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	join := func(a, b int32) {
		if ra, rb := find(a), find(b); ra != rb {
			parent[ra] = rb
		}
	}
	for i := 0; i+2 < len(p.Tris); i += 3 {
		a, b, c := same[p.Tris[i]], same[p.Tris[i+1]], same[p.Tris[i+2]]
		join(a, b)
		join(b, c)
	}
	group := map[int32]int{}
	var pieces []MeshPart
	remap := make([]int32, n)
	for i := range remap {
		remap[i] = -1
	}
	for i := 0; i+2 < len(p.Tris); i += 3 {
		g := find(same[p.Tris[i]])
		k, ok := group[g]
		if !ok {
			k = len(pieces)
			group[g] = k
			pieces = append(pieces, MeshPart{})
		}
		q := &pieces[k]
		for _, v := range p.Tris[i : i+3] {
			if remap[v] < 0 { // a vertex is in one piece only
				remap[v] = int32(len(q.Pos) / 3)
				q.Pos = append(q.Pos, p.Pos[3*v], p.Pos[3*v+1], p.Pos[3*v+2])
			}
			q.Tris = append(q.Tris, uint32(remap[v]))
		}
	}
	if len(pieces) <= 1 {
		return []MeshPart{p}
	}
	sort.SliceStable(pieces, func(a, b int) bool { return len(pieces[a].Tris) > len(pieces[b].Tris) })
	for i := range pieces {
		pieces[i].Name = fmt.Sprintf("%s %d", p.Name, i+1)
	}
	return pieces
}

// OnBed moves a part so it stands on z = 0, centred over x = y = 0 (as a
// slicer puts it on the plate; its turn is left as it is).
func OnBed(p *MeshPart) {
	if len(p.Pos) == 0 {
		return
	}
	lo := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	hi := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for i := 0; i < len(p.Pos); i += 3 {
		for k := 0; k < 3; k++ {
			lo[k] = min(lo[k], p.Pos[i+k])
			hi[k] = max(hi[k], p.Pos[i+k])
		}
	}
	d := [3]float32{-(lo[0] + hi[0]) / 2, -(lo[1] + hi[1]) / 2, -lo[2]}
	for i := 0; i < len(p.Pos); i += 3 {
		for k := 0; k < 3; k++ {
			p.Pos[i+k] += d[k]
		}
	}
}

// WritePartSTL writes a part as binary STL.
func WritePartSTL(w stdio.Writer, p MeshPart) error {
	bw := bufio.NewWriter(w)
	var head [80]byte
	copy(head[:], "Forma "+p.Name)
	bw.Write(head[:])
	binary.Write(bw, binary.LittleEndian, uint32(p.Triangles()))
	var rec [50]byte
	put := func(off int, x, y, z float32) {
		binary.LittleEndian.PutUint32(rec[off:], math.Float32bits(x))
		binary.LittleEndian.PutUint32(rec[off+4:], math.Float32bits(y))
		binary.LittleEndian.PutUint32(rec[off+8:], math.Float32bits(z))
	}
	for i := 0; i+2 < len(p.Tris); i += 3 {
		a, b, c := p.Tris[i]*3, p.Tris[i+1]*3, p.Tris[i+2]*3
		ax, ay, az := p.Pos[a], p.Pos[a+1], p.Pos[a+2]
		ux, uy, uz := p.Pos[b]-ax, p.Pos[b+1]-ay, p.Pos[b+2]-az
		vx, vy, vz := p.Pos[c]-ax, p.Pos[c+1]-ay, p.Pos[c+2]-az
		nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
		if l := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz))); l > 0 {
			nx, ny, nz = nx/l, ny/l, nz/l
		}
		put(0, nx, ny, nz)
		put(12, ax, ay, az)
		put(24, p.Pos[b], p.Pos[b+1], p.Pos[b+2])
		put(36, p.Pos[c], p.Pos[c+1], p.Pos[c+2])
		if _, err := bw.Write(rec[:]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadSTLPart reads an STL file as one part (named after the file).
func ReadSTLPart(path string) (MeshPart, error) {
	f, err := os.Open(path)
	if err != nil {
		return MeshPart{}, err
	}
	defer f.Close()
	tris, err := ReadSTL(f)
	if err != nil {
		return MeshPart{}, err
	}
	p := MeshPart{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	for _, t := range tris {
		for _, v := range []struct{ X, Y, Z float64 }{{t.A.X, t.A.Y, t.A.Z}, {t.B.X, t.B.Y, t.B.Z}, {t.C.X, t.C.Y, t.C.Z}} {
			p.Tris = append(p.Tris, uint32(len(p.Pos)/3))
			p.Pos = append(p.Pos, float32(v.X), float32(v.Y), float32(v.Z))
		}
	}
	return p, nil
}

// SeparateResult is a file written by SeparateParts.
type SeparateResult struct {
	Path      string
	Triangles int
}

// SeparateParts reads a 3MF's parts (or an STL as one part), optionally
// breaks each into its loose pieces, stands each on the bed and writes it
// as an STL of its own into dir (made if need be). say is told how it goes.
func SeparateParts(path, dir string, loose bool, say func(string)) ([]SeparateResult, error) {
	if say == nil {
		say = func(string) {}
	}
	say("reading " + filepath.Base(path))
	var parts []MeshPart
	switch strings.ToLower(filepath.Ext(path)) {
	case ".3mf":
		var err error
		if parts, err = Read3MFParts(path); err != nil {
			return nil, err
		}
	case ".stl":
		p, err := ReadSTLPart(path)
		if err != nil {
			return nil, err
		}
		parts = []MeshPart{p}
		loose = true // an STL's parts are its pieces
	default:
		return nil, fmt.Errorf("%q: pick a .3mf or .stl file", filepath.Ext(path))
	}
	if loose {
		var all []MeshPart
		for _, p := range parts {
			all = append(all, LoosePieces(p)...)
		}
		parts = all
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var out []SeparateResult
	used := map[string]int{}
	for i, p := range parts {
		say(fmt.Sprintf("writing %d of %d: %s", i+1, len(parts), p.Name))
		OnBed(&p)
		name := fileName(p.Name)
		if used[strings.ToLower(name)]++; used[strings.ToLower(name)] > 1 {
			name = fmt.Sprintf("%s (%d)", name, used[strings.ToLower(name)])
		}
		fp := filepath.Join(dir, name+".stl")
		f, err := os.Create(fp)
		if err != nil {
			return out, err
		}
		if err := WritePartSTL(f, p); err != nil {
			f.Close()
			return out, err
		}
		if err := f.Close(); err != nil {
			return out, err
		}
		out = append(out, SeparateResult{Path: fp, Triangles: p.Triangles()})
	}
	return out, nil
}

// fileName makes a part's name safe as a file name on every system.
func fileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	s = strings.TrimRight(s, ". ")
	if s == "" {
		s = "part"
	}
	return s
}

// parseDisplayColor reads a 3MF colour, "#RRGGBB" or "#RRGGBBAA".
func parseDisplayColor(s string) ([3]float64, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 && len(s) != 8 {
		return [3]float64{}, false
	}
	n, err := strconv.ParseUint(s[:6], 16, 32)
	if err != nil {
		return [3]float64{}, false
	}
	return [3]float64{float64(n>>16&255) / 255, float64(n>>8&255) / 255, float64(n&255) / 255}, true
}

// Model3MFSize is the size of a 3MF's model (its XML, unpacked), read from
// the zip's directory without unpacking anything.
func Model3MFSize(data []byte) (uint64, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("not a 3MF (zip) file: %v", err)
	}
	var n uint64
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".model") {
			n += f.UncompressedSize64
		}
	}
	return n, nil
}
