# Known limitations

What Forma can't do yet, or does only roughly, for working on later. Each has
an ID to refer to; strike one through (or delete it) when it's fixed, and add
new ones as features land.

## Kernel and booleans (`brep/`)

- **K1 Coplanar faces aren't merged.** After a join, two faces on one plane
  stay two faces with an edge between them (two boxes side by side keep a
  line across their shared top). Needs a merge pass after sewing: faces on
  the same surface sharing an edge become one, the edge and any vertex left
  with two edges go.
- **K2 Intersection curves are polylines.** Where a plane meets a cylinder
  square on (a boss's root, a hole's rim) the edge is a polyline, not a
  circle. Fine for display and booleans, but STEP export (M6) wants exact
  curves: recognise plane–cylinder, plane–cone, plane–sphere and coaxial
  cases and make lines, circles and ellipses.
- **K3 Speed.** A boolean takes up to about a quarter of a second, so
  dragging an extrude's arrow or the move gizmo on a joined body stutters,
  and a long history regenerates slowly. Ideas: bounding-box culling per
  face pair before contouring, coarser contour grids refined near the curve,
  caching tessellations, regenerating on release while dragging.
- **K4 Tangent contact.** Surfaces touching along a line or curve (a cylinder
  lying on a plane, a sphere in a cylinder of its radius) only work where the
  touching curve is also an edge. A general case is likely to fail.
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
  curved bodies) and one density for the whole design.
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

- **U1 No native file dialogs** (paths are typed).
- **U2 Not tried with a real mouse yet:** everything was checked by scripted
  captures; the feel of orbiting, picking, sketching and the gizmo needs a
  real session.
- **U3 Tangent edges** (where a round meets the face beside it) are drawn as
  dark lines like sharp edges; Fusion draws them lighter or hides them.
- **U4 Old capture scripts** that click toolbar buttons now inside drop-downs
  (`btn:create.box`, `btn:sketch.xy`, …) no longer find them; they'd need to
  open the menu first or call the command.
- **U5 Section panel's radio buttons** look odd (empty circles beside X / Z).

## Exchange (`io/`)

- **E1 STL only** (in and out). STEP, OBJ and assemblies are milestone M6.
