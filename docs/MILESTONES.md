# Milestones

What each milestone added, in the order they were built. The current gaps are in
[LIMITATIONS.md](LIMITATIONS.md).

## M0–M2: shell, kernel and sketcher

- **App shell** (`app/`, `ui/`, `render/`): docked layout (Browser, Viewport,
  Properties, Timeline), a ribbon toolbar (`app/ribbon.go`: icon-over-name
  tools in captioned groups — File, Sketch, Create, Modify, Pattern, Inspect,
  View; primitives and sketch planes in drop-downs; its own set in a sketch), status bar, Ctrl+K command palette, F1
  shortcuts. Open, Save as, Import and Export use Windows' own file
  dialogs (`github.com/ncruces/zenity`, modal to the window, run off the main
  loop; captures keep an in-app path box). Every action is an `app.Command`, shared by menus, toolbar,
  palette, shortcuts and capture scripts.
- **Viewport**: MSAA render target shown as an ImGui image; orbit round the
  point under the cursor (right drag or Shift+middle), pan (middle), zoom to the
  cursor (wheel); ImGuizmo view cube; standard views (numpad), zoom to fit,
  orthographic; infinite grid; ID-buffer picking (an R32UI target: faces,
  edges, vertices, sketch points, curves and profiles, with a few pixels of
  slack for points and lines).
- **Kernel** (`geom/`, `brep/`, `ops/`): float64 geometry in mm; half-edge
  B-rep (body → shell → face → loop → half-edge → edge → vertex), an edge-loop
  builder and Euler operators (MVFS, MEV, MEF, KEMR, KFMRH), `brep.Check`
  (topology, Euler–Poincaré, geometry on surfaces); watertight tessellation
  (shared edge samples, faces meshed in parameter space by a constrained
  Delaunay triangulation, seams and poles unwrapped); primitives Box, Cylinder,
  Cone / frustum, Sphere, Torus. Tests check validity, genus, watertightness
  and closed-form volumes and areas.
- **Sketcher** (`sketch/`): points, lines, circles, arcs, construction
  geometry; coincident, on-curve, horizontal, vertical, parallel,
  perpendicular, tangent, equal, concentric, midpoint, fix; distance, angle,
  radius, diameter dimensions. Levenberg–Marquardt solver (per-constraint
  numerical Jacobian), degrees of freedom and per-entity "fully constrained"
  from the Jacobian's row space, redundant / conflicting constraints refused,
  dragging, region (profile) detection with holes and exact areas. In the app:
  line chains, rectangles, circles, arcs, points, dimensions; snapping to
  points, midpoints, curves, centres and the grid; inferred horizontal /
  vertical; badges and dimensions drawn over the view; double-click a
  dimension to type a value or expression (`expr/`: `20mm + 5`, `1in`, `45deg`).
- **Exchange** (`io/`): binary / ASCII STL in, binary STL out (STEP and OBJ: M6).

## M3: features and history

- **Extrude and revolve** (`ops/extrude.go`, `ops/revolve.go`): any sketch
  region, holes included; extrude one side, symmetric or two sides; revolve
  about the sketch's X / Y axis or one of its lines, any angle (caps on a part
  turn, poles where the profile touches the axis). Lines become planes,
  cylinders or cones and arcs cylinders, spheres or tori. Each makes a new
  body; join / cut / intersect need booleans (M4).
- **The design** (`doc/`, headless): named parameters (expressions using each
  other, cycles reported), and a history of features (primitives, sketches,
  extrudes, revolves) whose inputs are expressions. Regeneration is
  incremental: a feature whose definition, parameters and inputs haven't
  changed keeps its results. Sketch dimensions can be expressions of
  parameters.
- **Persistent references**: faces are named by what made them (`top`,
  `bottom`, `side:<entity>:<n>`, `rev:<entity>:<n>`, `start`, `end`), a sketch
  on a face finds its face by name, and a profile is found by the sketch
  entities round it, so edits upstream carry through.
- **Timeline** with a rollback marker (step back / forward, roll to here,
  suppress, delete), undo / redo of every step (Ctrl+Z / Ctrl+Y, snapshots of
  the design), `.forma` files (JSON of parameters and history; Ctrl+S /
  Ctrl+O), autosave every minute while there are unsaved changes (File →
  Recover autosave).
- **In the app**: select profiles and press Extrude (Shift+E) or Revolve
  (Shift+R); drag the extrude's arrow to set its distance; edit any input in
  Properties; the Parameters window (toolbar); sketches used by a feature are
  hidden.

## M4: booleans

- **`brep.Boolean(a, b, Union | Subtract | Intersect)`** (`brep/bool*.go`), on
  the analytic surfaces: intersection curves are found by contouring one
  face's signed distance over the other's parameter grid (marching squares),
  pulled exactly onto both surfaces and densified to the tolerance, and kept
  as polyline edges (straight lines where both faces are planes); edges of
  one body lying on a face of the other (a boss standing on a plate, a pocket
  from a face) are imprinted on it. Each face is split in its own parameter
  space (seams and poles included) by region tracing; each region is classed
  inside / outside / on the other body by a generalized winding number just
  either side of it; the kept regions are sewn with edges shared by origin,
  disconnected results get a shell each. Failures come back as errors, never
  as broken solids. Tests: boxes, a tunnel, holes, a boss, a pocket, a blind
  hole, half spheres, the Steinmetz solid, and a fuzz of boxes and cylinders
  checked by V(A∪B) + V(A∩B) = V(A) + V(B) and V(A−B) = V(A) − V(A∩B).
- **In the design**: every body-making feature (extrudes, revolves,
  primitives) has an Operation — New body, Join, Cut, Intersect — applied to
  the bodies it touches; the design's bodies run through the timeline
  (`doc.BodyRef`: made by one feature, changed by later ones). Face names are
  qualified by the feature that made them (`f12/top`), so a sketch on a face
  of a joined body still finds it. An extrude sketched on a face joins by
  default; set to Cut it goes into the body.
- Seam edges (a face meeting itself) aren't drawn.
- Touching bodies: edges of the two bodies that run along each other (boxes
  side by side, a part joined to its mirror image at its own face) become one
  edge (`brep/bool_overlap.go`); curves starting on, ending on or touching a
  face's boundary are clipped right; two cylinders of one radius whose axes
  cross are cut by their two planes (no saddle for the contouring). Tests: a
  fuzz of 120 boxes on a coarse grid (flush, stacked, inside each other) and
  flush cylinders.

## M5: modelling tools

- **Transforms** (`geom/xform.go`, `brep.Transform`): rigid motions and
  reflections of a whole body (loops turned round under a reflection).
- **Move** (gizmo): turn about a pivot (X, then Y, then Z) and move; with the
  feature picked, ImGuizmo's arrows (or rings) in the view set its inputs.
- **Mirror** and **patterns** (linear, also as a grid, and circular, about X /
  Y / Z or the axis of a picked round face): of bodies (joined, or new
  bodies) or of features — an extrude, revolve, hole or primitive picked in
  the timeline is put in again at each place with its own operation, so a
  patterned hole cuts.
- **Fillet and chamfer** (`ops/fillet.go`): straight edges between flat faces
  and circles where a flat face meets a cylinder or cone square on (including
  the boolean's edges, a boss's root). The corner between the edge and the
  round (or bevel) is swept along it — extruded, or revolved for circles — and
  cut away (convex) or filled in (concave). Edges meeting at an angle meet in
  a mitre; a smooth chain (a top edge all round a part with rounded corners)
  flows round. Pick edges, or a face for all its edges.
- **Hole** (`ops/hole.go`): simple, counterbored or 90° countersunk, to a depth
  (with a 118° drill point or flat) or through all, at the points of a sketch.
- **Shell** (`ops/shell.go`): the body less a copy of itself with every face
  moved in by the thickness (open faces moved out), vertices and edges
  re-found where the moved faces meet; open faces picked, or none for a
  closed hollow.
- **Measure**: pick one thing (length, radius, area, position) or two
  (distance, exact for parallel faces and point–face, and the angle).
  **Mass properties** (`brep.Mass`): volume, area, mass by density, centroid,
  inertia tensor and principal moments. **Section view**: a clip plane on X,
  Y or Z with a slider; the cut is hatched.
- Faces the tools make are named for references: `round:<k>`,
  `chamfer:<k>`, `hole:<k>:…`, `in/<face>` (shell), `<k>/<face>` and
  `m/<face>` (pattern and mirror copies). Edges are referred to by the faces
  either side and a point near them.
- Limits: fillets on edges between curved faces (two cylinders, a sphere)
  aren't there yet; three rounded edges at a corner meet in mitres, not a
  spherical patch; a shell thicker than a feature (a round smaller than the
  wall) is refused.
- Everything known not to work yet is listed, with IDs, in
  [LIMITATIONS.md](LIMITATIONS.md).

## M6: exchange

- **STEP export** (`io/step_write.go`, File → Export, Ctrl+E, by the file's
  extension): AP214 (`AUTOMOTIVE_DESIGN`) advanced B-rep solids in mm — planes,
  cylinders, cones, spheres and tori with their exact parameters, lines and
  circles; intersection edges are fitted to circles or lines where their
  points lie on one, else written as degree-1 B-splines; cavities as
  `BREP_WITH_VOIDS`. Also STL and OBJ (`io/obj.go`, a group per face).
- **STEP import** (`io/step_parse.go`: an ISO 10303-21 reader; `io/step_read.go`;
  File → Import, Ctrl+I): the solids of AP203 / AP214 / AP242 files on those
  surfaces become bodies of the design (an Import feature with an Operation,
  its faces named `imp:<n>`, the file kept inside the `.forma`), units read
  from the file (inches, metres …); ellipse and B-spline edges become
  polylines pulled onto both faces. They take part in booleans, fillets and
  the rest like any body.
- **STL import** makes a reference mesh: shown, with its size, area and
  volume, not part of the solids.
- Tests (`go test ./io/`): every surface and edge kind written and read
  back (counts and volume); files written by OpenCASCADE read in
  (`io/testdata/`, made by `tools/stepcheck/make.py`: fillets with spherical
  corners, an oblique hole, crossing cylinders, a torus cut, a chamfered
  cone); with `FORMA_FREECAD` set to FreeCAD's `bin/python.exe` our files are
  read by OpenCASCADE and checked valid with the same volumes
  (`tools/stepcheck/check.py`).

## M7: assemblies

- **Components** (Assemble → Component): another `.forma` design inserted and
  placed (Move / Rotate inputs or the gizmo), kept inside this one; its
  bodies stay apart (`BodyRef.Comp`, with the body in the component's frame
  and its placement). Copy places another instance sharing the design;
  Update from file re-reads it.
- **Joints** (pick a face, edge or vertex on the part that stays, then one on
  the part that moves): rigid, revolute, slider, cylindrical. The moving
  part's frame (a flat face's centre and normal, a round face's axis, a
  circle's centre and axis — a boolean's polyline rims are fitted —, a
  line's middle) is brought onto the fixed one face to face (Flip: the same
  way), then turned by the angle and slid by the distance; the motion has a
  slider and Play.
- **Interference** (Clash): the overlap of every pair of bodies from
  different components, listed with its volume and drawn in red.
- **STEP**: components export as an assembly (a product per design, a
  `NEXT_ASSEMBLY_USAGE_OCCURRENCE` and transform per instance, as
  OpenCASCADE writes them), and assembly placements are applied on import.
  Tests: `doc/assembly_test.go` (a pin jointed into a plate's hole, flip,
  motion, instances, a reload, a face joint, interference) and `io`
  (OpenCASCADE's assembly read, ours written and read back by both).

## M8: kernel exactness

- **Exact intersection edges** (`brep/bool_exact.go`, `geom/conic.go`):
  where a boolean's two surfaces meet in a line, a circle or an ellipse, the
  edge in the result is that curve, not a polyline through the contoured
  points: a plane square to a cylinder, cone or torus, a plane through a
  sphere, two surfaces of revolution on one axis, straight runs (two planes,
  a cylinder cut along its axis), and a plane cutting a cylinder at a slant
  (`geom.Ellipse`). Circles are snapped to their surfaces (a drilled hole's
  rim has the drill's radius to the last digit). The faces are still split
  along the polyline; other curves (two crossing cylinders) stay polylines.
  Arcs run over [0, t1] from their start (`geom.StartAt`), as the
  primitives' circles do. STEP writes `ELLIPSE` and reads ellipse edges
  exactly.
- **Coplanar faces merge** (`brep/merge.go`, after every boolean): faces on
  one plane facing the same way that share an edge become one face (holes
  and all), and edges running on through a vertex with nothing else at it
  (two lines on one line, two arcs of one circle) become one. Two blocks
  side by side are a plain block.
- Tests: `brep/exact_test.go` (a hole's rims, a fillet on one, a slanted cut,
  a sphere's slice, crossing cylinders keeping their polyline),
  `brep/merge_test.go` (side by side, an L, a frame round a hole, drilling
  across a merged join), `io` (a slanted cut through OpenCASCADE, the oblique
  hole's ellipses read back).

## M9: splines (and dragging)

- **Curves and surfaces** (`geom/bspline.go`, `geom/sweepsurf.go`): clamped
  non-rational B-splines (`geom.BSpline`; `geom.Bezier` is the one-span
  case) with evaluation, derivatives, projection, length, de Casteljau
  splitting and chord-bounded sampling; `geom.Revolved` (a profile curve
  turned about an axis) and `geom.Extruded` (a curve swept along a
  direction), with the parameters and normals of STEP's surfaces of
  revolution and linear extrusion. Their signed distance stops at the
  profile's ends (a profile ending flat on a plane doesn't put the plane
  at distance 0). Transforms, uniform scaling (`brep.Scale`) and meshing
  handle them.
- **Sketcher**: a Spline entity (a Bézier on ordinary sketch points: its
  ends and handles drag, snap and take constraints like any point); the
  Spline tool (B: start, two handles, end; it chains on, the next first
  handle mirrored and held tangent); tangency between a spline and a line,
  arc or spline at a shared end; profiles cut splines exactly where curves
  cross (numeric intersection, de Casteljau pieces) with exact areas.
- **Features**: extruding a spline makes an extruded face, revolving one a
  revolved face (`ops/extrude.go`, `ops/revolve.go`); booleans on them. A
  revolved face against faces on its own axis (planes square to it,
  coaxial cylinders, cones, spheres, tori, other revolved faces) is
  intersected exactly by walking its profile (`brep/bool_coaxial.go`):
  crossings and tangent touches give circles.
- **Kernel fixes found on the way** (they failed before M9 too): a cut
  ending exactly where two edges cross (the ring's split line and a
  revolved foot's seam) now splits both; an imprinted edge half a
  millimetre inside a long cylindrical face is no longer dropped as "on
  its boundary" (a boss flush with a plate's side and a little taller); a
  sliver lying almost on a face of the other body is classified with a
  probe step shorter than the gap.
- **STEP**: B-spline edges, surfaces of revolution and of linear extrusion
  written and read exactly (OpenCASCADE reads them valid with the same
  volumes).
- **Dragging** (`app/snapping.go`, `app/handle.go`, `app/transform_ui.go`):
  drags snap to an increment (half the grid's spacing, or a fixed length
  from the status bar's Snap menu or `snap.step`), Ctrl moves freely,
  Shift is fine control (a tenth of the step and of the motion), with a
  readout by the cursor; sketch drags (which ignored the grid) included.
  Arrows on the selected feature resize it: an extrude's distance, a box's
  sides, a cylinder's radius and height and so on. A Resize · Move · Turn
  · Scale bar over the view: a primitive moves and scales by its own
  inputs, anything else gets a move feature (which now has a Scale), and
  Resize goes back from a move feature to the feature that made the body.
  The sketch snaps to its origin and pins what is drawn there; changing a
  dimension moves the loosest geometry first.
- **Sketch transform box** (`app/sketch_xform.go`): round a sketch selection
  of more than a point, with the Select tool; inside it (on a selected
  curve) moves the selection, a side handle stretches it along that axis
  about the opposite side, a corner scales it evenly about the opposite
  corner; snapped like every drag. Fixed points and plain-number
  dimensions within the selection take the new places and sizes.
- Tests: `geom/bspline_test.go`, `sketch` (spline regions, tangency),
  `ops/spline_test.go` (extruded and revolved splines against exact
  volumes, booleans through them), `ops/bowl_test.go` and
  `ops/bowlprobe_test.go` (a dog bowl's foot joined eight ways),
  `brep/scale_test.go`, `io` (spline parts through OpenCASCADE).

## M10: text and projection

- **Text** (`sketch/text.go`, `app/text_ui.go`): a Text entity placed by
  the ribbon's Text tool and edited in Properties: the words (several
  lines), font, height (the capital height), angle and alignment. Fonts are
  the TrueType and OpenType files in `assets/fonts` and the system's font
  folders (`golang.org/x/image/font/sfnt`), with kerning from the font's
  `kern` table. The letters' outlines (lines, quadratic and cubic Béziers)
  are sketch curves: profiles find their regions by non-zero winding, so
  counters (the holes in o, a, e) are holes and overlapping strokes make one
  region. Picking a text, or any region of it, takes the whole text as the
  profile, so an extrude of it still takes every letter after an edit.
- **Text parameters**: a parameter can hold text (`"Luna"`, in quotes), and
  a sketch text can show one (`=name`): change the parameter and every text
  showing it, and everything built on them, follows (also the `text.set`,
  `text.font` and `text.size` commands).
- **Projection** (`doc/project.go`): the ribbon's Project tool brings a
  body's edge into the sketch: a line as a line, a circle or arc square to
  the sketch as one, any other curve as a chain of lines. Its points are
  fixed (and a circle's radius) and it is drawn purple. Every rebuild
  projects the edges again, so the sketch follows the bodies it was drawn
  from; if an edge is gone, its geometry stays where it was and the sketch
  says so.
- Tests: `doc/text_test.go` (a name parameter carried through an extrude,
  counters as holes), `doc/project_test.go` (a box's edge and a cylinder's
  rim followed through edits).

## M11: emboss and inlay

- **Emboss feature** (`doc/emboss.go`, `ops/emboss.go`; ribbon Modify →
  Emboss with a text, or its sketch picked in the browser, and one face):
  the profiles are projected along the sketch's normal onto the face
  (where the line through a point meets it on the normal's side; "Land on
  the far side" for the other) and **raised** from it, **engraved** into
  it, or **inlaid**: a pocket per letter grown by the clearance, the depth
  deep, and the letters as bodies of their own sitting flush in it, the
  clearance under them too (a name inlaid in a bowl and printed in another
  colour). Depths are along the face's normal. With a text parameter, a
  new name rebuilds pockets and letters.
- **Extrude between surfaces** (`ops/extrude_to.go`): a region swept along
  its normal from one surface to another, built face by face (the caps'
  edges are the outline projected onto them), so a letter is made without
  a boolean; the caps are the face's surface moved along its normal
  (exact for planes and the conics, the offset of the stretch of a
  revolved Bézier the letter covers).
- **Outline offset** (`sketch/offset.go`): segments moved sideways
  (Béziers to within 1 µm), corners rounded where the pieces part and
  chorded where they overlap, the region by the non-zero winding rule;
  a text's `Grow`.
- **Kernel fixes found on the way:** the triangulator recovers its
  constrained edges by walking along them (it scanned every edge per
  flip: meshing a face with a letter cut in took 24 s, now 0.2 s, and it
  left cracks); a recognised line or conic must lie on both surfaces,
  not only fit the cut's points (a cone cut by a plane is a hyperbola,
  nearly straight); a two-point cut is a straight edge only if its chord
  lies on both surfaces; a hole is never put in the region its own loop
  bounds the other way round; a region is classified at the triangle
  centre deepest inside it; a short cut across a narrow face (a letter's
  short segment) is kept; the intersection grid goes on the much smaller
  of two curved faces, and its step is finer than the narrower face;
  split pieces of an edge end where their welded vertices lie.
- Tests: `sketch/offset_test.go` (squares, arcs, text grown and shrunk),
  `ops/emboss_test.go` (a name cut into, raised on and inlaid in a
  cylinder; inlaid in the dog bowl's slanted side; a letter at five
  places round a cone), `doc/emboss_test.go` (the name parameter
  followed, the far side, Cooper Black in all three modes).

## M12: ribbed forms, sketch offset, faster booleans

- **Faster booleans** (K3): the triangulator keeps each triangle's
  neighbours instead of a hash map of edges, links a new fan in one pass
  and tests inside / outside against the segments bucketed in bands; the
  winding numbers that classify regions come from a tree of triangles,
  far clusters taken as one dipole (Barill et al., fast winding
  numbers); a region's sample point is searched biggest triangle first;
  the grown text's sketch tests only the curve pairs whose boxes meet.
  A plate drilled by a cylinder: 51 → 17 ms; the test suites 2–3× faster
  (ops 71 → 27 s); a name in Cooper Black inlaid in a cylinder 15 → 2.1 s,
  raised or engraved 2.6 → 0.8 s.
- **Many tools in one boolean** (`brep.Gather`): letters that stand apart
  are cut or raised together (two booleans for a name, not one a letter),
  and a pattern's copies that stand apart and touch a body go in together
  (`Document.applyCopies`), each group falling back to one by one if it
  fails. 24 fins round a bowl: 1 s → 0.09 s.
- **Sketch offset** (`sketch/offset_sketch.go`, `sketch/offset_chain.go`;
  Sketch ribbon → Offset, or O): the selected lines, arcs and splines
  (a chain in any order, or a closed loop) or a circle copied a distance
  to the side — out from a loop, left of a chain, the other side for a
  negative distance. The copy is driven by an Offset constraint (a
  dimension, so it takes an expression) and follows its sources after
  every solve; its points are locked. Corners: lines meet at a mitre (an
  arc round the corner past four distances), any other outside corner is
  rounded, inside corners trimmed back to where the pieces cross.
  Deleting a curve of the copy deletes the offset; deleting a source lets
  the copy go free.
- **Clearance on any inlay:** a non-text profile's pocket is its region
  grown the same way (`sketch.GrowRegion`: outline out, holes in).
- **Ribbed bowl:** a fin sketched against a revolved bowl's wall, extruded
  symmetrically and joined, patterned round the axis
  (`captures/ribbed.json`, `captures/data/ribbed.forma`); on a cone wall
  24 fins take 0.09 s, on a revolved spline wall 1.7 s (its cuts are
  traced, K2b).
- **Kernel fixes found on the way:** a cut starting exactly on a seam goes
  into the face from the seam's other copy when that is where it heads,
  and so does one passing through a point right on a seam (a sphere
  centred in a cylinder's seam plane; a fin on a revolved wall's seam
  failed); a crossing of a face's edge that falls exactly on a point of
  the cut is counted once (a fin at 180° failed); a closed cut starting on
  the face's boundary is started where it is clear of it instead (on a
  spline-walled bowl every second fin failed: the circle its bottom cuts
  round the wall started along the first fin's edge on that circle); a
  hole goes in the smallest region round it by the area inside its
  outline (holes taken off as they were found made a ring look smaller
  than the region inside it).
- Tests: `brep/winding_test.go` (the tree against the exact sum, a
  sphere on a cylinder's seam, benchmarks), `sketch/offset_sketch_test.go`
  (rectangle out and in following a dimension, a slot past its radius, an
  open L, a circle, a spline, nested holes), `doc/ribbed_test.go` (24
  fins on a cone wall and on a spline wall, one fin every 22.5°),
  `doc/emboss_test.go` (a plate with a hole inlaid with clearance).

## M13: assembly

- **Grounding** (Properties → Grounded, or the command palette): a
  grounded component stays where it is placed. A joint whose moving end is
  on it moves the other end instead, a Move of it is refused, and the
  gizmo is off; a joint between two grounded components is an error. The
  first component inserted is grounded (as Fusion does), shown with a pin
  in the browser.
- **Joint limits** (Properties → Motion → Limits): a range for the motion
  input (a revolute joint's angle, a slider's distance; a cylindrical
  joint's angle). The joint holds its input within it and says so
  ("Distance held at its limit, 10 mm"); the motion slider runs over the
  range and Play swings back and forth between the limits.
- **Bill of materials** (Assemble → BOM; `doc.BOM`, `doc.WriteBOM`): a
  row per part, components counted by their file and design (copies, and
  the same file inserted again, are one line), then the design's own
  bodies; quantity, bodies, volume, area, and mass each and in all at the
  density set there. Export CSV writes it.
- **Exploded view** (Assemble → Explode; `doc.Explode`): a spread slider
  pulls the assembly apart for drawing only — each component along the
  axis of the joint that places it, away from what it is joined to (and
  further with that part), loose ones out from the middle, grounded ones
  staying. Picking still works on the parts where they are drawn, and Fit
  frames them. The renderer takes an offset per draw (`render.Paint.Offset`,
  `uOffset` in the vertex shaders).
- Tests: `doc/assembly_m13_test.go` (grounding swaps a joint and refuses a
  move; a slider held at its limit; the BOM with a copy, the same file
  again and a body of the design's own, and its CSV; the explode
  directions). Capture `captures/m13.json`.

## M14: static stress analysis

- **Volume mesh** (`fea/mesh.go`, `geom/delaunay3.go`): points on the
  part's faces (the surface mesher at the element size, with points
  inside flat faces too, `brep.MeshOptions.Interior`), a jittered
  body-centred lattice inside kept half an element clear of the surface
  triangles, and a sparser layer outside so the part's surface is never
  the hull; their 3D Delaunay tetrahedralisation (Bowyer–Watson with
  neighbour links, points in Morton order); the tetrahedra whose centre
  is inside the part (`brep.Winding`) kept and made quadratic, the middle
  of each boundary edge lying on a face moved onto its surface (and back,
  should that turn an element inside out). Boundary triangles know the
  B-rep face they lie on, which is how supports and loads find the mesh.
- **Solver** (`fea/elem.go`, `fea/solve.go`): 10-node tetrahedra with a
  4-point rule, linear isotropic elasticity, the stiffness in 3 × 3 node
  blocks, conjugate gradients with block-Jacobi preconditioning on all
  the cores; nodes on fixed faces held, a force spread over the loaded
  faces by area or a pressure along their normals (a 6-node triangle's
  share on its edge mids); von Mises stress worked out at every element's
  nodes and averaged, the maximum stress and displacement, and the safety
  factor against the material's yield (`fea.Materials`: steels,
  aluminium, brass, printed PLA / PETG / ABS, nylon, oak).
- **In the app** (Inspect → Stress, `app/simulate_ui.go`): select faces
  and hold them fixed or load them (a force in N or a pressure in MPa),
  pick a material and an element size (auto by default), Run (on a
  goroutine, with a progress bar); the part is drawn as its boundary,
  moved by the displacement (5 % of its size at ×1, a slider scales it)
  and coloured by von Mises stress or displacement with a legend
  (`render.ColorMesh`, `DrawColored`); the numbers and the safety factor
  beside it.
- Checks (`fea/fea_test.go`): a bar in tension (the stretch within 0.3 %
  of FL/EA, the stress 10 MPa away from the ends), a cantilever (tip
  deflection 0.19052 mm against beam theory's 0.19048, the bending stress
  at mid-span), a cylinder's mesh volume, a plate with a hole (3.1 × the
  nominal stress beside the hole, Kirsch's 3). Capture `captures/m14.json`.

## M15: electronics and behaviour

- **An ATmega328P** (`avr/`, written from the datasheet and the AVR
  instruction set manual after the design of avr8js): the whole AVRe+
  instruction set with its cycle counts, interrupts (the instruction after
  SEI or RETI first), the three ports with pull-ups and outside drive,
  timers 0, 1 and 2 in every mode (normal, CTC, fast and phase-correct
  PWM; their compare outputs on D3, D5, D6, D9, D10, D11; overflow and
  compare interrupts — what `millis()`, `analogWrite()` and the Servo
  library use; 16-bit registers through TEMP), the serial port (sends,
  receives, its interrupts), the ADC (13 ADC clocks a conversion, left
  adjust) and INT0 / INT1; Intel HEX firmware (`avr.LoadUno`), the Uno's
  pin names (`avr.D`, `avr.A`). Peripherals are clocked in 2 µs batches,
  caught up before any I/O register is touched, and interrupts are looked
  for only after something they depend on changes: a busy sketch runs at
  about 3.7× real time.
- **Parts** (`elec/`): a sketch stepped with what it drives — a battery
  (sagging over its resistance), an H-bridge (L298N / TB6612: the mean of
  IN1 − IN2 times EN's PWM), DC gear motors (back-EMF and torque from the
  rated voltage, no-load speed and stall torque; the speed solved exactly
  over a step; the TT motor), hobby servos (the last pulse's width, 544–
  2400 µs, as the angle, slewing at their speed; the SG90), LEDs, buttons
  (pull-up, pressed low) and potentiometers on the ADC.
- Tests: `avr/avr_test.go` (flags of ADD / SUB / 16-bit pairs / MUL /
  ADIW / SBIW, a loop's 31 cycles, calls and the stack, Timer 0's
  overflow rate, PWM duty, ports, serial, the ADC, HEX), `elec/elec_test.go`
  (a servo on Timer 1's 50 Hz PWM, a TT motor at half PWM, a button lit
  LED, a pot through the ADC) — programs encoded by hand — and compiled
  sketches (`avr/testdata/sketches`, rebuilt by `avr/testdata/build.ps1`
  with arduino-cli and the arduino:avr 1.8.8 core; the `.hex` files are
  kept, so the tests need no compiler): `blink` (the core's start-up,
  `millis()` 49 after `delay(50)` as on a board, `micros()`, `Serial`),
  `servo` (the Servo library: an SG90 at 45°, then 135°), `motor`
  (`analogWrite` on timer 0 into a TB6612: half speed, then full reverse)
  and `pot_led` (`analogRead` onto timers 1 and 2's PWM, a button with
  `INPUT_PULLUP`).
- **In the design** (`doc/circuit.go`): a circuit feature ("Electronics")
  holds the board, its firmware (the sketch's Intel HEX, kept in the
  `.forma` file) and library parts with their pins, a driver's supply and
  motor, and for a servo or motor the joint its shaft drives (a ratio and
  the shaft's angle at the joint's zero; a slider takes mm per degree). It
  makes no bodies; regenerating checks the wiring (unknown pins, a pot off
  the analogue inputs, two parts on one pin, a gone or rigid joint, two
  parts on one joint) and the joints it drives count as what it depends on.
- **The parts library** (`elec/library.go`): SG90 and MG996R servos, TT and
  N20 gear motors, TB6612 and L298N drivers, 4 × AA and 2S packs, a pot, a
  button, an LED — each with its pins, its figures and their defaults; a
  new part takes the usual free pin for its kind (servos and enables on
  PWM pins, pots on A0…). **3D models** of them (`doc/partlib.go`): the
  Uno's board with its holes, jacks, headers and chip, the servos with
  their tabs and shafts, the gear motors, the AA holder, the 18650 pack,
  the driver boards, and (added later) a 6 × 6 mm push button, a 16 mm
  pot and a 5 mm LED — built from primitives, inserted as components
  (Add part inserts the part's model too, unless its checkbox is off).
- **Motion study** (`doc.Study`): the sketch on the emulated board with its
  parts, run in whole 1 ms steps; after each run the driven joints take
  their shafts' angles (a revolute one wrapped to ±180°) and the design
  regenerates; Reset puts the joints back as they were.
- **The app** (`app/electronics_ui.go`): the ribbon's Circuit button opens
  the Electronics window — firmware (`.hex`, or `.ino` compiled with
  arduino-cli on a goroutine without a console window), Run / Pause /
  Reset and a speed, each part as it runs (servo angle, motor speed and
  current, driver voltage, LED brightness), a slider for each pot and a
  box for each button, the serial monitor, and the parts with their
  wiring, joints and figures; the circuit's summary in Properties.
  Commands `electronics.*` (add, firmware, part, wire, drive, set, model,
  run, pause, reset) for the palette and captures.
- Tests: `doc/circuit_test.go` (the servo sketch turning a pin in a plate
  by −45°, then +45°; a TT motor behind a TB6612 turning it through a 2:1
  ratio; the circuit saved and loaded; wiring errors), `doc/partlib_test.go`
  (every model one valid solid of its module's size, the shafts where they
  say), `elec/sketch_test.go` `sweep_drive` (an arm sweeping 20–160°, a
  wheel whose speed is a pot and whose direction a button). The demo
  assembly (`captures/data/arm_and_wheel.forma`) is written by
  `TestElectronicsDemo` with `FORMA_WRITE_DEMO` set, with a wired copy
  (`arm_and_wheel_wired.forma`) to open and run. Capture
  `captures/m15.json`.

## M16: drawings

- **Views** (`drawing/view.go`): a view looks at the design's bodies along
  a frame (right, up, the way it looks). Every B-rep edge is sampled, and
  every curved face's **silhouette** is found as the zero line of n·D over
  its own triangles (the mesh's normals are the surface's), chained into
  polylines. Each line is then split into **visible and hidden** runs. Its
  points are tested against all the bodies' triangles, projected and kept
  in a 2D grid. A change of state is bisected to a hundredth of a sample
  step, and lone flips are smoothed out. A silhouette is found a hair
  inside the true outline, where its own face's nearer facets stand far in
  front of it, so a cylinder's, cone's or sphere's outline ignores its own
  face: a line of sight touching it doesn't meet it again. Other faces
  count only from 2 % of the part's size in front. Lines lying on more
  important lines are dropped: visible edges, then silhouettes, tangent
  edges, hidden edges, hidden silhouettes, nearest first. A hole's far rim
  goes under its near one, and a cylinder's outline under an edge. Seams
  are left out. Tangent edges are kept as thin lines and can be switched
  off. A circle seen end on is kept as a circle.
- **Sections** (`drawing/section.go`): the bodies are cut by a plane with
  the kernel's booleans (the bodies ∩ a half-space box). A plane lying
  along a face is retried a hair beyond it. The faces on the plane come
  back as regions, hatched at 45° every 2.5 mm, even–odd between their
  loops. Neighbouring bodies alternate direction and spacing.
- **Sheets** (`drawing/sheet.go`): ISO A4–A0 and ANSI A–E, landscape or
  portrait, third or first angle projection.
  - **Frame:** 20 mm on the filing side and 10 elsewhere, zones marked
    every 50 mm, and centring marks.
  - **Title block:** title, number, material, finish, drawn and checked by,
    date, revision, company, scale, size, units and the projection symbol.
  - **Views:** base views (front, back, top, bottom, left, right and four
    isometric). Projected views and sections are seen from a side of
    another view, by the projection's rule, and stay lined up with it.
    They are placed clear of it from the bodies' extent, and a view's
    centre is the bodies' middle, so every view of them lines up.
  - **Standard sheet:** front, top, right and an isometric view, at the
    largest ISO 5455 scale the three orthographic views fit at. The
    isometric view takes its own smaller scale in the corner they leave,
    and is labelled with it.
  - **Centre lines:** a cross on every hole or boss seen end on, and a
    chain line down the axis of every cylinder or cone seen side on. Only
    for faces that go at least three-quarters of the way round, so a
    rounded corner gets none.
  - **Sections:** each gets its cutting line (thin chain, wide ends) with
    arrows the way it looks, its letter, and a SECTION A-A label.
  - **Line types:** after ISO 128 (wide 0.5 mm, narrow 0.25, dashed hidden
    lines, long-dash-dot centre lines).
- **Dimensions** (`drawing/dims.go`): horizontal, vertical, aligned,
  radius, diameter and angle, plus notes.
  - **Anchors:** a dimension is anchored to an edge by the names of the
    faces either side of it and a model point, as fillets are, or to a
    silhouette by its face. An anchor means the edge itself, its end, or
    its circle's centre. That is how they follow an edit: moving the boss
    or changing a parameter changes the number.
  - **Measuring:** two parallel lines are measured square to them, and a
    circle with a horizontal dimension gives its diameter.
  - **Drawing:** extension lines with gaps and overshoot, arrowheads
    inside, or outside when the span is narrow, and text readable from the
    bottom or the right. Radius and diameter dimensions use a leader and a
    shoulder. Text can stand in for the value (`<>` is the value:
    "4× Ø5 C'BORE Ø<> × 3").
- **Export:** written from one page of paths (with arcs where they are
  circles), fills and text laid out with Helvetica's metrics.
  - **SVG** in millimetres: a class per pen, true circles and arcs.
  - **PDF**: one page at full size, flate-compressed, with the base fonts
    Helvetica and Helvetica-Bold in WinAnsi, so nothing is embedded.
  - **DXF R12**: a layer per pen with CONTINUOUS / DASHED / CENTER line
    types, CIRCLE and ARC, polylines, SOLID arrowheads, TEXT with `%%c`
    and `%%d`.
- **In the design** (`doc/drawings.go`): `Document.Drawings` keeps the
  sheets: their views, dimensions, notes and title block, never their
  lines. They are saved with the design and are part of undo. Older Forma
  versions read the file and ignore them.
- **The app** (`app/drawing_ui.go`): the File group's Drawing button opens
  the design's first sheet, or a new A3 with the standard views.
  - **The sheet** is drawn on the viewport with ImGui's draw list: lines at
    their pens' widths and dashes. Text is placed glyph by glyph from the
    font atlas, so it turns to any angle, and is stretched to the width the
    page laid it out at.
  - **Ribbon:** Finish, New sheet (by size), Fit, Base view, Projected and
    Section (by side), the six dimension tools, Note, and Export (PDF, SVG,
    DXF).
  - **Navigation:** the wheel zooms about the cursor, and middle or right
    drag pans.
  - **Editing:** a click selects a view, dimension or note, and dragging
    moves it (a view takes the views seen from it along). Double-clicking a
    view zooms to it.
  - **Dimension tools:** these show what they would pick under the cursor
    (an edge, an end, a centre), then the dimension itself at the cursor
    before it is placed.
  - **Panels:** the Browser lists the sheets, views and annotations with
    their values. Properties edits the sheet (size, orientation,
    projection, scale, decimals, the title block), a view (direction,
    scale, hidden lines, tangent edges, centre lines, where a section
    cuts), a dimension's text and a note.
  - **Commands:** `drawing.*` (open, sheet, base, proj, section, dim, note,
    adddim, select, set, export) for the palette and capture scripts.
    Capture coordinates `sh:<x>,<y>` (sheet mm) and
    `dv:<view>:<x>,<y>,<z>` (a model point in a view).
- **Tests:**
  - `drawing/view_test.go`:
    - a plate with a hole seen from the front: 140 mm visible and the
      hole's 20 hidden;
    - the same plate from the top: the rim known as a circle, the far rim
      under the near one;
    - a cylinder side on and end on;
    - a boss's outline visible;
    - a section through the hole: 440 mm² cut, and its hatching.
  - `drawing/sheet_test.go`:
    - the side frames for both projections;
    - scales;
    - the standard layout lined up and inside the area, and a big part
      going down to 1:5;
    - six dimensions measuring 60, 15, Ø16, R10, 25 and 90° and following
      the boss moved and made bigger;
    - a section sheet;
    - every page written as SVG (parsed as XML), PDF (each xref offset at
      its object, the content stream inflated, every text there) and DXF
      (group codes, entity counts).
  - `doc/drawings_test.go`: a sheet saved and read back with the design,
    and its dimension following a parameter.
  - Captures: `captures/m16.json` (the tools driven by clicks, a section,
    a note, dragging, export) and the README's `drawing` shot.

## After M16: selection, sketches on faces, the sketch's code

- **Sketches on curved faces** (`doc.TangentPlane`): a sketch on a
  cylinder, cone, sphere or torus face lies on the plane touching it where
  the face was clicked (`PlaneRef.At`), facing out, its X round the
  axis and its Y along it, so a text on it stands upright; Sketch is on
  for any face picked. **Moving a sketch** (`Document.Placed`): every
  sketch has Offset (along its normal), Turn (about it, degrees), Move X
  and Move Y (along its turned axes), expressions like any input; "Put on
  the selected face" moves it onto another face or place.
- **Emboss off a face's side:** a letter running over the side of the face
  as the sketch sees it (where the face turns away and its moved copy is
  missed) is refused with a plain message (`ops.ErrOffSide`) instead of a
  boolean's topology error.
- **Selecting several things** (`app/multiselect.go`): Ctrl+click adds in
  the view, the browser and the timeline; several steps picked together
  show their shared values in Properties, an edit setting them on all
  (and hide, suppress or delete them together); a drag on empty space is
  a box picking the sharp edges it shows (Shift: the faces; read from the
  ID buffer, `render.PickTarget.ReadRect`); double-clicking an edge picks
  its tangent chain (`brep.TangentChain`), a body all of it (Ctrl: as
  well); fillet and chamfer with bodies picked in the browser and nothing
  in the view round all their sharp edges (`brep.Sharp`: no seams, no
  tangent edges).
- **The sketch's code** (`app/code_editor.go`): the Electronics window's
  Write code opens an editor (ImGui Color Text Edit: C++ highlighting,
  undo, find; Consolas) whose text is kept with the circuit
  (`Circuit.Source`); examples to start from (Blink, Servo sweep,
  Button and LED, Motor speed from a pot); Compile and load builds it
  with arduino-cli on a goroutine (`compileCode`), loads the firmware and
  marks the compiler's errors and warnings on their lines, listed under it
  (a click goes there); Open in VS Code writes it to
  Documents/Forma/sketches/<name>/<name>.ino and opens it there, and a
  save there is read back.
- **Combine** (`KCombine`, `doc.NewCombine`): bodies already made
  joined, cut or intersected with a target body (the first picked, the
  others the tools), the tools used up or kept (Keep tools); bodies that
  don't meet are refused plainly.
- **X-ray** (`Feature.XRay`): any body drawn see-through (no depth
  writes, back faces culled), its edges dashed over everything; picks go
  through it to what is behind. From the body's Properties, its context
  menu or View → X-ray.
- **Text wrapped round a face** (Emboss's Wrap, `ops.EmbossSpec.Wrap`):
  on a cylinder or cone each letter goes to the angle its place along the
  line gives (x / radius round the axis), turned to face out there, so a
  line longer than half the way round reaches the back; inlaid pockets
  that grow into each other are matched to their letters
  (`pocketOf`), and a letter landing on the seam is nudged and tried again.
- **Arduino parts you can touch:** holding the mouse on a button's 3D
  model presses it while a study runs; an LED's lens glows with its
  pin's duty; the Electronics window has a wiring table (part, pin, Uno
  pin). **arduino-cli from Forma** (`arduinocli`): when it isn't installed,
  Forma downloads Arduino's release for the system into
  %LOCALAPPDATA%\arduino-cli\bin and installs the AVR core and the Servo
  library, with a progress bar.
- Also: sketch regions with overlapping straight pieces (Inter's D, whose
  outline runs back along itself) made doubled loops, which broke emboss
  and inlay on that letter; the duplicate is now dropped
  (`sketch/overlap_test.go`). A new step made with several things picked
  shows its own properties.
- **Kernel fixes from two pet bowls** (a tapered two-colour bowl with
  oval hand holes and its name wrapped round, and a ribbed bulb with its
  name raised, both from photos; `doc/bowls_test.go`):
  - a hole across a tapered wall failed: an extruded circle is two half
    cylinders, and the cut round the hole comes back to each half a turn
    on, in a copy of its parameters the trim never looked at; it now
    enters a face's next copy across its edge there (`clip`);
  - splitting a body along a wave failed: past a spline face's curve the
    distance to it jumped in sign, which the contour followed as a curve,
    and real curves stopped short of the face's edge; extruded and
    revolved splines are carried on past their ends along the end
    tangents (`geom.evalOn`, `projectOn`), and contour points that don't
    lie on both surfaces are dropped;
  - a name on a revolved spline failed at every size: the same, on the
    offset faces the letters are built between, and a letter's flat foot
    on the circle the letters before it left on the bowl was trimmed
    against the whole bowl first (`turnShareAbout`: a plain face's share
    of the turn is the angle its points span);
  - a contour crossing a face's edge at the very start of a segment
    shorter than the weld tolerance was taken for a touch, and a contour
    starting just past an extruded face's edge for one starting on it;
  - text running off the side of a face is refused plainly again,
    rather than silently losing the part past the side.
- **Ribs through a raised name (K8):** three ribs of 48, 60 or 72 failed
  where they passed a hair from a letter's corner. A crossing within the
  weld distance of its edge's end is now that end (at 285° a rib's side
  missed the A's top corner by 0.6 µm, and its crossings of the three
  edges there made three vertices too close for any cut to join), and an
  exact edge whose vertex was welded off it more than Check allows is a
  toleranced polyline; a boolean that fails is tried again at finer
  tolerances (`Boolean`: ½, ¼, 1⁄10; never coarser, which could weld a real
  feature away), which settles an edge passing 0.02 mm from another; a
  pattern's group of copies that fails goes in as halves, not one by one
  (`applyCopies`; the groups without retries, `brep.BooleanOnce`).
  `TestRibByLetterCorners`.
- **A tutorial video** (`captures/tutorial_bowls.json`, recorded with
  FORMA_CAPTURE's `record`): both bowls built in the app's own UI step by
  step with captions; `text.align` (left, centre, right) for scripts.
- **Linux and macOS builds** (`tools/release/build.sh`, the release workflow's
  linux and macos jobs and a release job gathering all three): Linux a
  tar.gz (X11, `-tags x11`), macOS Forma.app with one universal binary
  (built for arm64 and amd64, joined by lipo), its icon made from the app's
  PNGs, signed ad hoc, in a zip. The file dialogs' owner window is
  Windows-only (`dialog_owner_*.go`); VS Code is found in its macOS and
  Linux places. `arduinocli.TestRealDownload` (`FORMA_ARDUINO_DOWNLOAD=1`)
  installs arduino-cli from Arduino into an empty folder and runs it.
- Also: the view cube drew the camera upside down and slid the target off
  the part when a face of it was clicked; the ribbon tightens, drops its
  names, then scrolls in narrow windows; adding an electronics part puts
  its 3D model in (push button, pot and LED models added); picking a pin
  no longer crashes.
- Tests: `doc/sketch_place_test.go` (a sketch touching a cone, FORMA
  raised, engraved and inlaid on it, the sketch moved and saved; text off
  the side refused plainly), `brep/chain_test.go` (a rounded plate's rim
  a chain of 4 lines and 4 arcs, its tangent edges not sharp). Captures
  `cone_emboss.json`, `multiselect.json`, `multiselect2.json` (a chain
  chamfered), `code.json` (Blink compiled and blinking an LED, an error
  marked on its line), `cube_check.json`; `doc/combine_test.go` (two boxes
  joined, cut and intersected, the tool kept or not, saved and read
  back), `TestEmbossWrapsRound` (WRAPPED ROUND inlaid round a cylinder,
  letters on the back), `arduinocli/install_test.go` (the download and set-up
  against a local server); captures `combine.json`, `xray.json`,
  `press.json`, `wrap.json`, `installcli.json`.

## 0.3.0: printing

- **Split Body** (`ops.SplitBody`, `doc` kind `split`, `feature.split`):
  a body cut in two by a plane — XY, XZ or YZ moved along its normal (the
  command starts on XY through the body's middle), or a flat face of
  another body picked with it — keeping both (the body keeps the front, the
  back is a new body of the step) or one side. The halves are the body
  intersected with a half-space box each side of the plane; the new faces
  are named `f<ID>/split/front` and `/back`. A plane that misses the body
  or only touches a face says so. Mirror and Split share the plane lookup
  (`featurePlane`).
- **Colours:** any step has a colour (`Feature.Color`, `#rrggbb`, saved),
  picked in a body's Properties or set by `body.color:<#rrggbb>`, and
  drawn in the view.
- **3MF export** (`io.Write3MF`, File → Export… with a `.3mf` name): each
  visible body a part with its step's colour (core base materials), the
  parts one object of components so they keep their places, millimetres,
  each mesh's vertices welded within a micrometre so it is watertight.
  PrusaSlicer reads it (`--info`: manifold, the right volumes).
- Also in 0.2.x: X-ray parts can be clicked again (and the X-ray button
  with nothing selected turns every part solid); typing in a text rebuilds
  the design when the typing pauses; wrapped letters whose cuts meet under
  the surface are cut as one tool (engraved) or parted by a wall through
  the axis (inlaid).
- Tests: `ops/split_test.go` (a box split straight both ways, a cylinder
  slanting, a plane on a face or past the body refused),
  `doc/split_test.go` (Both / Front / Back, by another body's face, saved
  and read back, a colour saved), `io/threemf_test.go` (two bodies in two
  colours: the package's parts, names, colours, every mesh closed and as
  big as its solid), `ops/emboss_wrap_test.go`.
- **Separating a 3MF into part files** (`io.Read3MFParts`, `io.SeparateParts`,
  File → "Separate a 3MF into part files…" and "…into its loose pieces…"):
  the parts a 3MF's build puts on the plate (each object holding a mesh a
  part, an object of components a part for each, the build items' and
  components' transforms applied, names from the object or PrusaSlicer's
  `Slic3r_PE_model.config`), each stood on the bed and centred, written as
  binary STL into "<file> parts" beside it, on a goroutine with progress in
  a toast (captures wait for it). The model is scanned tag by tag (Go's XML
  decoder was far too slow for a gigabyte), meshes kept in float32: a
  PrusaSlicer project of 34 parts and 12.2 million triangles reads in about
  10 s. `LoosePieces` breaks a mesh into the pieces that share no vertex
  (an STL is always broken so). Test: `io/threemf_read_test.go` (our own
  3MFs apart and as components read back by name and volume, loose pieces,
  a build item turned and moved, PrusaSlicer's names, files on the bed).
- **3MF import into a design** (`doc.NewImport3MF`, an import feature with
  `Format` 3mf; `ops.MeshSolid`): each part the file puts on the plate a
  solid body named and coloured as the file has it (base materials,
  `io.MeshPart.Color`). `MeshSolid` welds the mesh, checks it is closed
  and manifold (else the part is left out, saying why), merges each flat
  region of triangles with one outline into one face (a box from our own
  export comes back with its six faces) and builds a polyhedral B-rep the
  kernel cuts like any other. The file is kept in the feature (as STEP
  text is); a model over about 60 MB (300,000 triangles) is refused from
  the zip's directory at once, pointing to Separate. Colours are now per
  body (`Feature.BodyColors`, `doc.ColorOf`: a body's own, else its
  step's, else its 3MF part's). Edges between flat faces meeting at under
  10° aren't drawn, so a faceted part doesn't look like a wire net. Tests:
  `ops/meshsolid_test.go` (a box's mesh back to six faces, a faceted
  cylinder cut by a box, an open mesh refused), `doc/import3mf_test.go`
  (a design's box and cylinder exported in two colours and imported into
  another: names, colours, six faces, volumes, split, saved; a picked
  colour beats the file's).

## M19: fillet upgrades (mostly done, in 0.4.0)

- **Ball corners** (`ops.CornerTool`, `ops.BallCornerOf`, `EdgeToolSet`):
  where three rounded edges of one fillet meet at a vertex of three flat
  faces (straight, convex edges; the vertex has no other edge), the corner
  is part of the rolling ball, not the mitre the rounds made crossing.
  The ball's centre C lies r in from all three faces; each edge's round
  stops at the plane through C square to the edge (where its cross-section
  is centred on C), and between the vertex and those planes a corner tool
  is cut first: built directly, not by booleans — three quadrilaterals on
  the faces, three end faces each with an arc of the ball cut out, and the
  spherical triangle between the points where the ball touches the faces
  (it only touches them, which booleans can't find). The rounds' end
  faces then fall exactly on the corner tool's, and the kernel takes them
  out. An edge too short for the corners at both its ends keeps mitres;
  chamfers keep them. Every edge of a box rounded is now the textbook
  rounded box (26 faces, 8 balls); a triangular prism's 60° corners too.
  Tests: `doc/fillet_corner_test.go` (a box's every edge and one corner by
  volume, a chamfer, a radius nearly half the box, a prism against the
  exact volume of the rolling-ball shape).
- **Chamfers by two distances, or a distance and an angle** (chamfer
  inputs Type, Distance 2, Angle, Swap; `doc.chamferSizes`,
  `ops.EdgeToolSet` with a distance for each side, `ops.ChamferSecond`):
  the first distance along the face each edge's reference names first (so
  it stays put through edits), the second along the other; an angle is
  the chamfer's from the first face, turned into the second distance by
  the sine rule with the angle the faces meet at (square faces: d·tan θ);
  Swap trades the faces for every edge. Chamfers from before have only a
  Distance and stay equal. Where mitred tools cross at a corner, one can
  trip the kernel (two passing exactly through the same point of an edge)
  that goes in cleanly once the others are in, so an edge whose cut fails
  waits and the rest go on (every edge of a box with two distances).
  Test: `doc/chamfer_test.go` (2 and 5 cutting ½·2·5·40 mm³, Swap, 30°
  from a face giving 2·tan 30°, an old chamfer, every edge, a cylinder's
  edge, an angle the faces can't take).
- **Edge sets with radii of their own** (`EdgeRef.Set`, inputs "Radius 2",
  "Radius 3"… — "Distance k" for a chamfer; `doc.SetEdgeSize`,
  `ClearEdgeSet`, `EdgeSets`, `SizeKey`; `app/edgesets_ui.go`, command
  `fillet.ownsize:<expr>`): a step's edges can be put in sets with a size of
  their own, picked by their rounds (a rounded edge is gone from the model;
  its round's face `f<ID>/round:<k>` names it) or as sharp edges added.
  Ball corners where three edges of one size meet (sets of the same value
  count as one size). Where sizes differ, the step rounds in passes,
  largest first (`roundEdges` once a size): their mitres would have touched
  each other along lines at the corners (both rounds tangent to the same
  face), which the kernel can't sort out in any order; from the second pass
  on each edge's round carries on along the edges running smoothly from it
  that the kernel can round (`brep.TangentChain`, `roundable`), which picks
  up the arcs the larger rounds left on the faces — rims round uprights
  rounded larger carry round their corners as tori. Tests:
  `TestFilletEdgeSets` (two sets at one size still balls; uprights at 8
  against the exact volume; a set cleared; saved), `TestFilletMixedRadii`.
- **Tangent chains followed** ("Tangent chain" input, "Follow tangent
  edges" in the properties; on for new fillets and chamfers, off for steps
  from before so they keep their shape): a round carries on along the edges
  running on smoothly from the picked ones (`brep.TangentChain`), those the
  kernel can round — one top edge of a box with rounded uprights rounds the
  whole rim. The passes of a mixed-size step and the one-size case are one
  loop now; the extra edges' rounds are numbered after the step's own.
  Test: `TestFilletTangentChain` (the rim with the chain, one edge without
  it or from before the option, a chamfer's chain).

## M20: screws and threaded holes (done; screws and screw holes in 0.4.0)

- **Screws** (`doc/fasteners.go`: `ScrewStds`, `ScrewSpec`, `ScrewDesign`,
  `NewScrew`; `app/fasteners_ui.go`, command `assemble.screw[:<std>,<size>,
  <length>]`, the Screw button in Assemble): ISO 4762 socket head cap, ISO
  7380 button head, ISO 10642 countersunk and ISO 4017 hex head screws, M2
  to M12 (coarse pitch) at their standard lengths, dimensions from the
  standards' tables. A screw is a component whose design is generated from
  its inputs (Screw, Size, Length; `buildComponent` rebuilds it), so editing
  them in Properties rebuilds it, and joints, explodes and the parts list
  take it as they are (two of the same are one row). Its frame: the axis
  along Z, the bearing face at z = 0 (a countersunk head: its top), the
  shank down to −Length. With a hole's rim selected (a circle with a flat
  face beside it), the screw is placed there, along the face's normal, and
  takes the size the hole was made for. Several rims selected: a screw in
  each.
- **Screws jointed in their holes** (`Document.ScrewInHole`, `holeSeat`;
  joint ends `JointRef.Frame` — a frame fixed in the component, the
  screw's seat looking down its shank — and `JointRef.Out` — a round
  edge's frame turned to look out of the part, along the flat face beside
  it): each screw the Screw command puts in a rim gets a revolute joint
  ("<screw> in its hole") from its seat to the hole's, so it follows the
  hole when the part moves or the hole is edited, and the joint's Angle
  turns it in the hole. A counterbore's top rim seats the screw on the
  counterbore's floor (the nearest round edge of the same hole step on the
  axis, deeper in, beside a flat face looking the same way).
- **Holes for screws** (hole inputs Screw, Fit; `ScrewHoleSizes`): a hole
  made for a size takes its diameter from the tables — clearance close /
  normal / loose (ISO 273) or the tap drill (nominal less one pitch) — and
  a counterbore or countersink sized for the screw's head.
- Tests: `doc/fasteners_test.go` (64 screws, every standard and size at two
  lengths, valid and as big as their parts; a screw component named,
  moved, resized and counted in the parts list; an M5 counterbored hole
  cutting the tables' volume with an M5×16 sitting in it touching nothing,
  a tapped and a countersunk hole's sizes; `TestScrewInHole`: socket,
  countersunk and button heads jointed into counterbored, countersunk and
  plain holes, seated without touching the block, following the hole's
  point when it moves, turned 30°, saved and reopened). Captures: two
  countersunk screws dropped into countersunk holes; two socket head screws
  jointed into counterbored holes in one go, following them when the block
  and the hole's sketch are raised.
- **Threads on drawings** (`doc/threads.go` `threadsOf`, `drawing.Thread`,
  `threadLines`; a view's Threads box, `View.NoThread`): ISO 6410's thin
  lines, worked out from the faces — a tapped hole's drill wall (a hole
  step with Fit Tapped) gets its thread at the nominal diameter, a screw's
  shank its thread at the minor diameter (d − 1.22687 P) from its tip, 2d +
  12 long on socket and countersunk heads with a wide line across where it
  stops. Side on, two lines along the thread; end on, three quarters of a
  circle open at the upper right. They go through the projection's hidden
  line test like edges (an internal thread is hidden in a plain view and
  shown in a section through it; an external one is never drawn hidden),
  and a section keeps only the part of a thread on its side.
- **Hole callouts** (dimension kind `hole`, the Hole tool in Annotate;
  Callouts puts one on every hole step seen end on in the selected view,
  over the view; `Document.HoleCallout`, `drawing/callout.go`): a leader
  to the hole's rim and a shoulder with its description — "2× Ø5.5 THRU",
  "Ø4.2 THRU" and "M5×0.8 THRU" for a tapped hole, "⌴ Ø10 ↧5.4" under a
  counterbore, "⌵ Ø11.4 × 90°" under a countersink, a depth "↧10" for a
  blind hole. The depth, counterbore and countersink symbols aren't in
  Helvetica, so they are drawn as lines (`richText`); a dimension's text
  replaces the callout, "<>" standing for it. Tests:
  `doc/threads_test.go` (the threads a block's tapped hole and a loose
  M5×30 get; the callouts; what top, front and section views show of
  them; callouts drawn on the sheet). Capture: a block with a tapped and
  two counterbored holes, a screw in one, its standard views, Callouts on
  the top view and a section through the tapped hole.
- **Washers and nuts** (`doc/nuts.go`: `WasherStds`, `NutStds`, `PartSpec`,
  `FastenerDesign`, `NewFastener`; commands `assemble.washer[:<std>,<size>]`
  and `assemble.nut[:<std>,<size>]`, the Washer and Nut buttons in
  Assemble): ISO 7089 plain and ISO 7093-1 large washers, ISO 4032 and thin
  ISO 4035 hex nuts, M2 to M12, components whose design comes from their
  inputs (Washer or Nut, Size), standing on z = 0 up to their thickness.
  Every screw, washer and nut put on a rim goes through
  `Document.FastenerInHole`: jointed onto the top of what is already
  stacked on that rim (`stackTop`: the joint holding something to the
  rim's seat, then up the joints on each one's top face, `climb`) or, with
  nothing there, onto the hole's seat; a washer's or nut's own rim picked
  stacks on it. The joints are named "… on …". Tests: `doc/nuts_test.go`
  (34 washers and nuts, valid and of their volume; a bolted joint through
  a plate — washer and screw on top, washer and nut, picked by the
  washer's rim, under it — at the right heights, clash free, following the
  hole, two washers one row of the parts list, no washer over a screw).
  Capture: that joint built in the app from the rims, seen in X-ray from
  the front, with its parts list.
- **Modelled threads** (`ops/thread.go`: `ThreadSpec`, `ThreadedRod`; the
  primitive `thread` (Diameter, Pitch, Height, Internal), joined or cut;
  input Thread = Modelled on a screw or nut component and on a tapped hole
  step, `ModelledThread`; the Thread choice in their properties): ISO
  68-1's basic profile swept along a right-hand helix, 48 flat facets a
  turn. A threaded length is built whole as a rod (the profile's band
  wound round the axis, clipped flat at both ends and capped by fans from
  the axis; shifted a fraction of a step so no vertex lies on an end's
  plane, or clipping left edges of no length) and made a solid by
  `MeshSolid`, then joined or cut with no facet crossing a face of the
  part but on a flat end: a screw's rod joined to the plain shank above
  it (its crests P/100 under the nominal so it sits inside the shank's
  rim), a nut's and a tapped hole's cut where a drill a hair under its
  crests went (`Crest`). On drawings the thread facets' edges and
  outlines are left out (`drawing.ThreadFacet`), a modelled thread drawn
  as its cosmetic thin lines plus its outline (`Thread.Outline`, the
  facets counting as one face that can't hide them), and a hole callout
  can be put on a modelled hole's outline circle. Tests:
  `ops/thread_test.go` (a rod joined to a shank under a head, cut from a
  drilled block), `doc/modelled_test.go` (a modelled M5×30 screw, nut and
  through tapped plate valid and taking the material they should; drawn
  with no facet edges, outlines and thin lines in a top view and a
  section; a callout on the outline). Capture: an M8 modelled screw, nut
  and tapped plate, and their drawing.
- Also: edges no longer show through faces in close-ups (the near plane as
  far out as the scene in view allows, `Camera.near`, and lines pulled
  toward the camera by a fraction of their distance, `render.depthBias`);
  a boolean cut through a face's corner crosses it once or not at all, and
  `MeshSolid` builds the same faces every run (`TestMeshSolid` failed now
  and then).
- **A video of what's new since 0.3.0** (`captures/whats_new.json`, recorded
  with FORMA_CAPTURE's `record`): two-distance and distance-and-angle
  chamfers, rounds with sizes of their own, tangent chains, holes made for
  screws and screws jointed into them, with captions. Capture targets of one
  name drawn twice in a frame take the first (`markItem`: two joints both
  "… in its hole"), and a joint's Play button is `btn:joint.play`.

## M21: circuit wiring (done)

- **Terminals and wires** (`elec/library.go`, `doc/wiring.go`): every
  library part has named terminals (an LED's A and K, a servo's SIG, V+
  and GND, a driver's IN1, IN2, EN, VM, GND, OUT1 and OUT2, a battery's +
  and −, a pot's 1, W and 2) and the board's are its pins, 5V, 3V3, GND
  and VIN. A circuit keeps `Wires`, each between two terminals
  ("uno.D9", "arm.SIG") with a colour and corners; wires that touch make
  nets. New parts: a resistor and a slide switch. A circuit made before
  wires (pins picked per role, a driver's battery and motor by name) is
  given the wires it meant the first time it is built (`migrate`): an LED
  a resistor of its own between the pin and its anode and its cathode to
  ground, a servo its supply, a pot its ends across 5 V, a button ground,
  a driver its battery and motor; adding a part wires it the same way.
- **The simulation follows the wires** (`elec/net.go`): `Build` makes a
  `Network`, the simulation's first part, which solves the nets each step
  by nodal analysis — the board's pins as 25 Ω sources (or a 35 kΩ
  pull-up, or nothing), batteries, resistors, pots, switches, LEDs (a
  forward voltage and 15 Ω, solved on or off until it settles) — and
  feeds the rest: inputs read their net's level, the ADC its voltage, an
  LED is lit by the current through it (a PWM pin's LED by mixing its
  pin high and low by the duty, not by the average voltage, which would
  barely clear its drop), a servo with no supply on V+ stands still, a
  driver's inputs are its nets' levels and its supply the battery on VM.
  `Check` finds shorts (5 V to ground, a battery's ends), an LED with no
  resistor, a motor on a pin, a servo's supply on a pin, pins wired to
  each other or to a rail, terminals left unwired.
- **The wiring sheet** (`app/wiring_ui.go`; Electronics → Wiring sheet,
  `electronics.sheet`): the Uno and each part as a box with its terminals,
  laid out in a column ordered by the board pins they are on (half a pitch
  off the board's, so no two terminals are level); wires routed square,
  each net's vertical run in a lane of its own clear of the boxes and of
  the other nets' runs (`sheetRoutes`); colours red on a supply's net,
  black on ground's, others by net or as picked. Drag a terminal onto
  another to wire them, a box by its title to move it; double-click a
  wire for a corner and drag it; right-click a wire for its colour,
  straightening or deleting it; Delete takes the selected wire off. The
  checks list under it as it is drawn; while the study runs the LEDs
  glow, a button's box held presses it, a switch's clicked turns it, and
  a terminal's tooltip gives its net and voltage. The Electronics window's
  wiring table still picks board pins (a terminal reaching the board
  through another part says through which), and each part's Wires list
  shows what every terminal is wired to (× takes a wire off) with a menu
  wiring it to any other part's terminal or the board's. Commands
  `electronics.connect` (`a=b[=colour]`), `electronics.disconnect`,
  `electronics.place` (`part=x,y`).
- **Wokwi** (`doc/wokwi.go`, `app/wokwi_ui.go`; `electronics.wokwi`): a
  project's `diagram.json` and the sketch beside it come in as a new
  circuit — the Uno as the board, LEDs, resistors (with their values),
  push buttons, servos, pots and slide switches named by their ids, every
  connection a wire keeping its colour, a breadboard's strips and rails
  joining what is wired to them — and its sketch is compiled; what was
  left out is listed on the sheet.
- Tests: `elec/net_test.go` (a divider read by the ADC, an LED and its
  resistor on 5 V, a battery lighting an LED through a switch, a button
  lighting an LED through the sketch, the pot_led sketch's PWM LEDs, the
  sweep_drive circuit wired by wires behaving as wired by name and its
  servo unpowered, the checks), `doc/wiring_test.go` (old wiring
  converted), `doc/wokwi_test.go` (a Wokwi project with a breadboard),
  `app/wiring_test.go` (no wire through a box, no two nets on one lane).
  Capture `captures/m21.json`: the converted arm-and-wheel circuit on the
  sheet, running, a wire drawn and the short it makes found, a Wokwi
  project opened, compiled and run.
- After it: the Delete key on a selected wire no longer crashes the sheet
  (the wire is taken out at the start of the next frame, `wireSheet.del`;
  capture `captures/m21_delete.json`); adding a part with its 3D model
  links the model to that part, not the last one added (an LED's model
  had gone to its resistor, so it never lit; `fixModels` repairs such
  files); a lit LED gives off its own light (`render.Paint.Glow`) on a
  curve the eye reads (`ledLook`); closing the window or Quit with unsaved
  changes asks to Save, Don't save or Cancel (`app/close_dialog.go`), and
  so do New design, Open and Recover autosave (`whenSaved`; a capture
  script's commands go straight through, `asked:<command>` asks).
