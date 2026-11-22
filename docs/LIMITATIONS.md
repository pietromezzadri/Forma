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
  sphere off a cylinder's axis, two crossing cylinders, a revolved or
  extruded spline face against a face not on its axis (a plane through a
  revolved spline's axis meets it in the profile itself; it is still
  contoured). The faces are
  split along the contoured polyline even where the edge is exact, so
  regions match the exact edge only to the contour tolerance.
- **K3 Speed** (M12: 2–3× faster, see MILESTONES). Still: an everyday
  boolean takes 15–60 ms, a body with many faces more, and every boolean
  meshes both bodies again from scratch, so a long history regenerates
  slowly. Ideas: caching face meshes between booleans, bounding-box
  culling per face pair before contouring, regenerating on release while
  dragging.
- **K4 Tangent contact.** Surfaces touching along a line or curve (a cylinder
  lying on a plane, a sphere in a cylinder of its radius) only work where the
  touching curve is also an edge. A general case is likely to fail.
  (M9: a revolved face against faces on its own axis is intersected
  exactly, profile against profile, so a foot leaving a ring's floor
  tangentially joins.)
- **K4b Nearly coincident faces fail.** Two cylinders of one radius whose
  axes are a hair apart (0.0002 mm: a ring's outside and a flange revolved
  onto it about an axis the ring's centre had drifted off) or a few mm
  apart (they cross along two lines at a grazing angle) make the join
  fail; exactly coaxial ones work. Sketches now keep a pinned origin and
  move the loosest geometry when a dimension changes, so this no longer
  happens by itself, but a centre placed off the axis by hand still
  hits it.
- **K4c Faces that only touch.** Found building the part models: joining a
  box to a cylinder when the box's top lies in the cylinder's cap and its
  edge crosses the cap (a horn's arm on its hub, both 3 mm tall: an edge
  "leaves face's surface by 1e-6 mm"), a box whose face runs through a
  cylinder's axis (the face "lost all of itself"), and two boxes meeting
  face to face with their tops level ("edge used twice the same way") all
  fail. Overlapping them by a fraction of a millimetre works.
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
  edges, a plane meeting a cylinder at a slant, spline edges or the edges
  of revolved or extruded spline faces (M9).
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
- **S4 Spline faces aren't offset:** a body with revolved or extruded spline
  faces can't be shelled (an offset spline isn't a spline; it needs fitting).

## Emboss and inlay (`ops/emboss.go`, `doc/emboss.go`)

- **EM1 Projected, not wrapped:** letters are projected along the sketch's
  normal, so on a curved face they spread a little toward its sides (a
  name 30 mm wide on a 40 mm radius is 3–4 % wider at its ends) and their
  walls lean against the face's normal there. No wrapping by arc length yet.
- **EM2 One face:** the letters must land on the one face picked; a name
  running over the edge between two faces (a bowl's side and its foot)
  fails ("the letters run off the face").
- **EM3 Faces it works on:** planes, cylinders, cones, spheres, tori and
  revolved one-span Bézier profiles (not extruded spline faces, nor
  revolved profiles of several spans).
- ~~EM4 Clearance needs text~~ (M12: any profile, grown by the sketch
  offset).
- **EM5 Speed** (M12: letters apart from each other go in together, two
  booleans a name): a Cooper Black name inlaid in a cylinder rebuilds in
  about 2 s; a face with many letters is still slower (K3).
- **EM6 Boolean robustness:** the cuts between the letters' walls and a
  curved face are traced on a grid, and a cut landing just so on its points
  can still fail; the letter is then tried again reaching a little further
  past the surface (up to four times). Very tight letter curves (a heavy
  font's small counter grown by more than its own radius) are offset
  roughly where the offset folds over.

## Holes, patterns, mirror, move

- **H1 Holes:** no threads (cosmetic or modelled; planned in M20), the countersink is always
  90° and the drill point 118°; holes go square into the sketch's face only.
- **P1 Linear patterns** run along X, Y or Z only (not along an edge or a
  sketch line); no pattern along a path; no skipping instances.
- **P2 Feature patterns and mirrors** repeat features that add or cut a shape
  (extrude, revolve, hole, primitive), not fillets, chamfers or shells.
- **M1 Move:** rotations are three angles (X, then Y, then Z), so the gizmo can
  jump near a 90° turn about Y (gimbal lock); the gizmo works in world axes
  only. Scale is uniform (a body scaled unevenly would turn circles into
  ellipses), and components can't be scaled.

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

- **SK1 Few tools:** no ellipses, slots, polygons, trim, extend or sketch
  fillets (offset: M12). Splines (M9)
  are single Bézier curves (quadratic or cubic) chained with tangency: no
  fit-point splines, no point-on-spline constraint, no dimensions on a
  spline's length or curvature.
- ~~SK3 No transform box~~ (M9: drag inside a selection to move it, a side
  handle to stretch it, a corner to scale it). Still: no rotate handle, and
  a circle in a stretched selection keeps its roundness (its radius by the
  mean of the two stretches).

- **SK2 Solver:** numerical Jacobian, one cluster; big sketches will slow down.
- **SK4 Text:** fonts are read from the system's font folders and
  `assets/fonts` (TrueType, OpenType, the first face of a collection);
  kerning only from a font's old `kern` table (not OpenType GPOS), no
  ligatures or right-to-left shaping, no text on a path, no bold / italic
  but by picking that font's file. Each letter extrudes as a body of its
  own, and the edges between a letter's curve segments are drawn dark
  (U3).
- **SK5 Projection:** edges only (no silhouettes of curved faces); a curve
  that isn't a line or a circle square to the sketch comes in as 24 lines;
  a projected circle that later becomes another kind of curve stops
  following (reported on the sketch).
- **SK6 Offset:** the copy is locked geometry worked out after each solve,
  not solved with the sketch (constraints can be put on it, but it can't
  be dragged); where an offset wider than a short piece swallows it the
  corner is joined by a chord, so the copy can cross itself; a copy whose
  make-up changes (a mitre turning into an arc, a piece vanishing) is made
  anew and loses the constraints on its old curves; no offsetting a text
  (texts have Grow) or a mix of a circle and other curves.

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
  planar or pin-in-slot); limits (M13) only hold the motion input within
  a range, they don't stop parts meeting; no contact, no motion study.
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
- ~~A6 No BOM, exploded views or grounding~~ (M13). Still: no rigid
  groups, no nested-component editing; the BOM counts parts by their file
  and design, so a part changed in one copy only is a separate line, and
  it has no part numbers, materials or descriptions of its own (one
  density for all).
- **A7 Exploded views** are drawn only: one spread for the whole
  assembly, each part along its joint's axis (or out from the middle), no
  steps, trails or saved views, and measuring, the section view and
  exports see the parts assembled.

## Stress analysis (`fea/`, `app/simulate_ui.go`)

- **FE1 Linear and static only:** small displacements, linear elastic
  isotropic materials, no plasticity (stress past the yield is reported,
  not followed), no buckling, contact, thermal, modal or fatigue studies.
- **FE2 One part:** a study takes one body; no assemblies, bolts or contact
  between parts, and no symmetry or remote supports — a face is either
  held fixed in all three directions or free.
- **FE3 Loads:** a force spread evenly over faces by area, or a uniform
  pressure; no point loads, bearing loads, moments, gravity or
  acceleration, and no loads varying over a face.
- **FE4 Mesh:** one element size over the whole part (no refinement near
  holes and fillets), set by hand or a quarter of the thinnest extent;
  the stress at sharp inside corners and at the edges of held faces grows
  without end as the mesh is refined (a real singularity: read stress a
  little away from them). Features much thinner than the element size
  may be lost; the mesher keeps tetrahedra by their centre, so a slot or
  gap narrower than an element can be bridged.
- **FE5 Speed:** the solver is conjugate gradients with a block-Jacobi
  preconditioner (a symmetric Gauss–Seidel one took 60 % fewer
  iterations but its sweeps don't run in parallel, so it was slower):
  5,000 elements take 1–4 s, 20,000 about 15 s. A multigrid or
  incomplete-Cholesky preconditioner, or a direct sparse solver, would
  make big studies practical.
- **FE6 Results:** von Mises and displacement only (no principal stresses,
  components, reaction forces or probes); the plot shows the outside of
  the part (no section through the result), and a study isn't saved
  with the design.

## Electronics (`avr/`, `elec/`, `doc/circuit.go`; M15)

- ~~EL1 Not in the design yet~~ (now a circuit feature in the `.forma`
  file, a motion study and the Electronics window in the app).
- **EL2 The chip:** an ATmega328P only (Uno, Nano). Not emulated: SPI,
  I2C (TWI), the EEPROM, the watchdog, pin-change interrupts, timer input
  capture, the analogue comparator, external clock sources, sleep modes
  other than idle. Timer flags and PWM edges come up to 2 µs late (the
  peripherals run in batches), and a 16-bit timer's compare registers take
  a new value at once rather than at TOP.
- ~~EL3 Not yet run on compiled sketches~~ (now five, compiled by
  arduino-cli with the arduino:avr 1.8.8 core: the core's start-up,
  `millis()`, `micros()`, `delay()`, `Serial`, the Servo library,
  `analogWrite()` on all three timers, `analogRead()`, `INPUT_PULLUP`).
  Still: libraries that talk SPI or I2C (displays, IMUs, drivers on a bus)
  won't run (EL2).
- **EL4 Parts are averages:** a motor sees its PWM's mean over a step (no
  inductance, current limit or driver heating), a servo has no load or
  overshoot, a battery is a voltage behind a resistance with no discharge.
- **EL5 Speed:** about 3.7× real time for a busy sketch.
- **EL6 The study is kinematic:** a shaft sets its joint, but the
  mechanism puts nothing back on it — a motor's load and inertia are its
  figures, not what it drives; a joint held at a limit doesn't stall the
  motor or the servo; no gravity, contact or a car rolling on the floor.
- **EL7 A study isn't kept:** no timeline of a run to scrub back through,
  no plot of a part over time, no export of the motion; a study ends when
  the circuit is edited, and the design's undo history sees the joints
  only once something else is committed while one runs.
- **EL8 Wiring is by name:** pins are picked per part, not drawn; no
  wires, no breadboard view, no reading a Wokwi `diagram.json` or a KiCad
  netlist; one board per circuit and an Uno only.

## Drawings (`drawing/`, `app/drawing_ui.go`; M16)

- **DW1 Hidden lines come from the tessellation:**
  - Something in front of a line by less than about 0.04 % of the part's
    size doesn't hide it.
  - Where a torus or spline face hides part of its own outline, only from
    2 % of the part's size in front.
  - The split between visible and hidden is found to a hundredth of a
    sample step (about 1/500 of the part's size).
- **DW2 Views:** base, projected and full sections only.
  - No detail views, broken views, auxiliary views at any angle, half,
    offset or aligned sections, or breakout views.
  - No exploded assembly view on a sheet.
  - A section cuts with one straight plane through the whole part.
- **DW3 Dimensions:**
  - Horizontal, vertical, aligned, radius, diameter and angle only. No
    ordinate, baseline or chain dimensions, no arc lengths.
  - Tolerances only as typed text, no GD&T frames or datums.
  - Nothing is dimensioned automatically.
  - The text sits on the line (no horizontal text on a vertical
    dimension). No snapping of a dimension's place to others.
- **DW4 Annotations:** no automatic hole callouts, no balloons or parts
  list for an assembly, no surface finish or weld symbols, no revision
  table.
- **DW5 One sheet per export:** each sheet is its own PDF, SVG or DXF (no
  multi-page PDF).
- **DW6 Text:**
  - Laid out with Helvetica's metrics, and written in Helvetica (PDF), a
    Helvetica / Arial family (SVG) or the reader's DXF font.
  - On screen it is the UI's font stretched to the same widths.
  - No choice of font, and no bold on screen.
- **DW7 DXF R12:** arcs only where a circle is seen end on. Ellipses and
  everything else are polylines.
- **DW8 Speed:** a view is worked out again from scratch when any of its
  bodies changes (cached otherwise). A big assembly takes seconds, and the
  app waits for it.
- **DW9 Hatching:** one pattern (45°, the direction and spacing alternating
  between bodies), no patterns by material.
- **DW10 Sections go through the booleans:** a plane lying along faces is
  retried a hair beyond it (K4b), and a body that still can't be cut is
  left out with a warning on the view.
- **DW11 Anchors are face names** (as D1): an edit that renames or splits
  the faces either side of an anchored edge loses the dimension, which is
  then listed with why it can't be drawn.
- **DW12 Placement:** new projected views and sections are placed by the
  bodies' extent, and nothing stops views, dimensions or notes from
  overlapping.

## Exchange (`io/`)

- ~~E1 STL only~~ (M6: STEP in and out, OBJ out, STL in as a reference mesh).
- **E2 STEP import reads analytic solids:** faces on planes, cylinders,
  cones, spheres and tori, and (M9) surfaces of revolution and of linear
  extrusion of non-rational B-spline curves. B-spline surfaces, rational
  swept curves and offset surfaces are refused (the solid is left out, with
  the reason); so are open shells and surface models. Lines, circles,
  ellipses and non-rational B-spline edges lying on their faces come in
  exact; other B-spline edges become polylines pulled onto both faces.

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
