# Forma

A parametric 3D CAD program in Go: constrained sketches and B-rep solids with
analytic geometry (planes, cylinders, cones, spheres, tori), drawn with OpenGL
4.1 under a docked Dear ImGui interface.

```bash
go run .                  # the app
go test ./...             # kernel, sketcher, STL, expressions (headless)
go run ./tools/stlcheck f.stl   # facets, volume, closed / oriented
```

Run from PowerShell on Windows (Git Bash loads the wrong DLLs). Needs a C
compiler for cgo (MSYS2 gcc works).

## What's here (milestones M0–M2)

- **App shell** (`app/`, `ui/`, `render/`): docked layout (Browser, Viewport,
  Properties, Timeline), a ribbon toolbar (`app/ribbon.go`: icon-over-name
  tools in captioned groups — File, Sketch, Create, Modify, Pattern, Inspect,
  View; primitives and sketch planes in drop-downs; its own set in a sketch), status bar, Ctrl+K command palette, F1
  shortcuts. Every action is an `app.Command`, shared by menus, toolbar,
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
- **Exchange** (`io/`): binary / ASCII STL in, binary STL out.

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
  [docs/LIMITATIONS.md](docs/LIMITATIONS.md).

## Screenshots without touching the mouse

`FORMA_CAPTURE=captures/basic.json` opens a hidden window that never takes
focus, runs each shot's actions (command IDs with `:args`, or `hover:fx,fy`,
`click`, `sk.tool:<name>`, `sk.click`, `wait:n`, `cam:yaw,pitch,dist`,
`sk.demo`, `export:<file>`, `log`) and writes `<out>/<name>.png`. See
`app/capture.go`.

## Roadmap

- **M3 features and history**: extrude (distance, symmetric, to face, through
  all), revolve, new body / join / cut; a feature tree with a timeline and
  rollback marker; a parameters table driving expressions; deterministic
  regeneration; undo / redo; `.forma` files storing the history; autosave.
  Persistent naming of faces and edges by their generator.
- **M4 booleans**: analytic face–face intersection, splitting, classification,
  merging; fuzz tests on volume identities.
- **M5 modelling tools** (done): fillet, chamfer, patterns, mirror, shell,
  holes, move / rotate with the ImGuizmo gizmo; measure, mass properties,
  section view.
- **M6 exchange and assemblies**: STEP AP214 export, OBJ, STL as reference
  meshes, components and joints.
- Ideas: Starlark scripting for code-driven parts, 2D drawings to SVG / PDF.

## Stack

Go, `go-gl/glfw` v3.4 (cimgui-go's ImGui GLFW backend is built against GLFW 3.4
and asks for 3.4-only window attributes, which v3.3 treats as fatal), `go-gl/gl`
4.1 core, `AllenDang/cimgui-go` v1.6.1 (Dear ImGui 1.93 docking, ImGuizmo),
using ImGui's own GLFW and OpenGL 3 backends fed from go-gl's callbacks. Fonts:
Inter and Font Awesome Free (OFL, `assets/fonts/CREDITS.txt`).
