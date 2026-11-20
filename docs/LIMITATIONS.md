# Known limitations

What Forma can't do yet, or does only roughly, for working on later. Each has
an ID to refer to; strike one through (or delete it) when it's fixed, and add
new ones as features land.

## Kernel and booleans (`brep/`)

- ~~K1 Coplanar faces aren't merged~~ (M8: `brep.MergeFaces` after every
  boolean). Still: **K1b curved faces aren't merged** — two pieces of one
  cylinder (a hole drilled into each of two blocks before joining them, a
  round split by a cut) stay two faces with a line between them, and arcs
  meeting such a line stay two arcs. Merging them needs the loops sorted
  into outer and holes in a periodic parameter space.
- ~~K2 Intersection curves are polylines~~ (M8: lines, circles and
  ellipses where the surfaces meet in one, `geom.ExactCurve`). Still:
  **K2b other curves stay polylines** — a plane cutting a cone at a slant
  (an ellipse, parabola or hyperbola), a plane along a cone's apex, a
  sphere off a cylinder's axis, two crossing cylinders. The faces are
  split along the contoured polyline even where the edge is exact, so
  regions match the exact edge only to the contour tolerance.
- **K3 Speed.** A boolean takes up to about a quarter of a second, so
  dragging an extrude's arrow or the move gizmo on a joined body stutters,
  and a long history regenerates slowly. Ideas: bounding-box culling per
  face pair before contouring, coarser contour grids refined near the curve,
  caching tessellations, regenerating on release while dragging.
- **K4 Tangent contact.** Surfaces touching along a line or curve (a cylinder
  lying on a plane, a sphere in a cylinder of its radius) only work where the
  touching curve is also an edge. A general case is likely to fail.
- **K4b Nearly coincident faces fail.** Two cylinders of one radius whose
  axes are a hair apart (0.0002 mm: a ring's outside and a flange revolved
  onto it about an axis the ring's centre had drifted off) or a few mm
  apart (they cross along two lines at a grazing angle) make the join
  fail; exactly coaxial ones work. Sketches now keep a pinned origin and
  move the loosest geometry when a dimension changes, so this no longer
  happens by itself, but a centre placed off the axis by hand still
  hits it.
- **K5 Non-manifold results are refused.** Bodies meeting only along an edge
  or at a point make no solid; the boolean reports an error.
- **K6 Equal crossing cylinders are a special case.** Two cylinders of one
  radius whose axes cross are cut by their two planes
  (`splitPlanes`); other degenerate pairs (equal cones, a sphere touching a
  cylinder) have no such help.

## Fillet and chamfer (`ops/fillet.go`)

- **F1 Only some edges.** Straight edges between flat faces, and circles where
  a flat face meets a cylinder or cone square on. Not edges between two
  curved faces (two cylinders, a cylinder and a sphere), torus or sphere
  edges, or a plane meeting a cylinder at a slant.
- **F2 No corner patches.** Three rounded edges at one corner meet in mitres
  (the rounds cross), not the ball-shaped corner Fusion makes. Needs a
  spherical (or setback) patch at vertices where three rounds meet.
- **F3 Tangent chains aren't followed.** Each edge must be picked (picking a
  face picks all its edges); Fusion follows tangent edges on by itself.
- **F4 Square ends.** A round stops square at an end where no other picked
  edge meets it at an angle. Where the face beyond that end is slanted, it
  leaves a sliver of the sharp edge; where it is carried on into a mitre past
  a slanted face, it can cut into it.
- **F5 Chamfer: equal distances only** (no two distances, no distance and
  angle). No variable-radius or chord-length fillets.

## Shell (`ops/shell.go`)

- **S1 Thin features.** The body less a copy with every face moved in; that
  copy keeps the body's topology, so a wall thicker than a feature (a round
  smaller than the thickness, a slot narrower than twice it) is refused
  rather than the feature closing up.
- **S2 Busy vertices.** A vertex where four or more faces meet that don't
  agree once moved (the tip of a pyramid) has no place and fails.
- **S3 One thickness.** No per-face thickness, no shelling outward.

## Holes, patterns, mirror, move

- **H1 Holes:** no threads (cosmetic or modelled), the countersink is always
  90° and the drill point 118°; holes go square into the sketch's face only.
- **P1 Linear patterns** run along X, Y or Z only (not along an edge or a
  sketch line); no pattern along a path; no skipping instances.
- **P2 Feature patterns and mirrors** repeat features that add or cut a shape
  (extrude, revolve, hole, primitive), not fillets, chamfers or shells.
- **M1 Move:** rotations are three angles (X, then Y, then Z), so the gizmo can
  jump near a 90° turn about Y (gimbal lock); the gizmo works in world axes
  only and doesn't snap.

## Measure, mass properties, section

- **I1 Measure:** distances involving curved faces or edges are approximate
  (sampled points, shown with "~"); exact only for two vertices, a vertex
  and a flat face, and parallel flat faces. Angles only between flat faces
  and straight edges. No edge–edge minimum distance.
- **I2 Mass properties** come from a fine tessellation (a part in 10⁴–10⁵ on
  curved bodies: a sphere's volume is 1.5 parts in 10⁴ low, where OpenCASCADE
  gets it exactly) and one density for the whole design.
- **I3 Section view:** X, Y or Z planes only (not a face or any angle); the cut
  is shown by hatching the inside faces seen through it, not real cap faces,
  so it looks wrong for open (non-solid) geometry.

## Sketcher (`sketch/`)

- **SK1 Few tools:** no splines, ellipses, slots, polygons, offset, trim,
  extend or sketch fillets; no projecting body edges into a sketch.
- **SK2 Solver:** numerical Jacobian, one cluster; big sketches will slow down.

## Design history and references (`doc/`)

- **D1 Edge references** are the names of the faces either side plus a point
  near the edge; if an upstream change splits or moves edges a lot, a fillet
  can land on a different edge between the same two faces, or report it
  gone.
- **D2 Split faces share a name:** after a boolean splits a face, the pieces
  keep one name, so a sketch on "top" takes the first piece.

## App and UI (`app/`)

- ~~U1 No native file dialogs~~ (Windows' own Open / Save As windows now, `app/native_dialog.go`; captures still type paths). Untested on macOS / Linux, where zenity needs zenity or kdialog installed.
- **U2 Not tried with a real mouse yet:** everything was checked by scripted
  captures; the feel of orbiting, picking, sketching and the gizmo needs a
  real session.
- **U3 Tangent edges** (where a round meets the face beside it) are drawn as
  dark lines like sharp edges; Fusion draws them lighter or hides them.
- **U4 Old capture scripts** that click toolbar buttons now inside drop-downs
  (`btn:create.box`, `btn:sketch.xy`, …) no longer find them; they'd need to
  open the menu first or call the command.
- **U5 Section panel's radio buttons** look odd (empty circles beside X / Z).

## Assemblies (`doc/assembly.go`, `app/assembly_ui.go`)

- **A1 Components are copies:** a component keeps the design as it was when
  inserted (inside this design); changes to its file come in only with
  Update from file, and its own history can't be edited in place (open its
  file, change it, save, Update).
- **A2 Joints** are rigid, revolute, slider and cylindrical only (no ball,
  planar or pin-in-slot); no motion limits, no contact, no motion study.
  Each joint moves its second component once, in timeline order: there is
  no solver, so loops (a four-bar linkage) can't be closed, and a component
  moved by two joints ends up where the later one puts it.
- **A3 Joint snap points:** a flat face's area centre, a round face's axis at
  the middle of the face, a circle's centre, a line's middle or a vertex;
  no picking other snap points (a face's edge midpoints, an end of an axis)
  and no offsets but the joint's own angle and distance.
- **A4 Features act on components' bodies** like any others: a cut through
  two components cuts both, and they stop being components (STEP then
  writes them as plain bodies).
- **A5 Interference** intersects the bodies pairwise: slow with many
  components, and a pin exactly the size of its hole (round faces lying on
  each other) may come back as "couldn't be checked" (K4).
- **A6 Not there yet:** a bill of materials, exploded views, grounding a
  component, rigid groups, nested-component editing.

## Exchange (`io/`)

- ~~E1 STL only~~ (M6: STEP in and out, OBJ out, STL in as a reference mesh).
- **E2 STEP import reads analytic solids only:** faces on planes, cylinders,
  cones, spheres and tori. B-spline surfaces, surfaces of revolution or
  extrusion and offset surfaces are refused (the solid is left out, with the
  reason); so are open shells and surface models. Lines, circles and
  ellipses come in exact; B-spline edges become polylines pulled onto both
  faces (fine for booleans; not exact).
- **E3 STEP import flattens assemblies:** placements (`NEXT_ASSEMBLY_USAGE_OCCURRENCE`
  transforms, at any depth, a part used many times) are applied, but the
  solids come in as bodies of one Import feature, not as components;
  `MAPPED_ITEM` instancing, colours, layers and names of faces aren't read.
- **E4 STEP export** writes components as an assembly (a part per distinct
  design, an occurrence per component) but no colours or names of faces.
  Lines, circles and ellipses are written as such; the polyline edges left
  (K2b) as lines or circles when their points lie on one (within 1e-5 mm),
  else as degree-1 B-splines through the points.
- **E5 STL import is a reference mesh only:** shown flat-shaded with its size,
  area and volume, but not pickable in the view, not measurable against other
  things, and never turned into a solid for booleans.
- **E6 OBJ:** export only, no materials.
- **E7 Imports keep the whole file in the design** (`.forma` files grow with
  big imports), and there's no "replace with a newer version of the file":
  import again and delete the old feature.
