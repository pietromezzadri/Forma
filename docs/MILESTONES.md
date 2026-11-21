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
