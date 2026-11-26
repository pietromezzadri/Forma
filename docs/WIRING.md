# Wires: the plan for M21 and M22 (Nov 2026)

Today a circuit is wired by name (see EL8 in [LIMITATIONS.md](LIMITATIONS.md)):
each part of a circuit step picks the Uno pins its roles go to
(`CircuitPart.Pins`, role → board pin), there are no wires to draw, and
parts reach each other only through the board — a button lights an LED
because the sketch reads one pin and writes another. Two milestones take
wires further: M21 makes the wiring itself part of the design, M22 puts
the same wires into the 3D assembly as cables.

## M21 circuit wiring

**What it gives.** A wiring view where wires are drawn from pin to pin,
parts that connect to each other and not only to the board (a button
straight to an LED, a battery through a switch to a motor driver), a
simulation that follows the wires, and Wokwi circuits brought in with
their wiring and their firmware.

- **The model.** A circuit step keeps a netlist: every part has named
  terminals (an LED's anode and cathode, a button's two sides, a
  battery's + and −, a driver's inputs, outputs and supply, the Uno's
  pins, 5V and GND as a part of its own), and `Circuit.Wires` lists wires
  between two terminals, each with a colour. Today's `Pins` become wires
  from the board when an older file is opened, so nothing breaks. Wires
  that touch make nets (joined by a union-find).
- **The simulation.** Each step, the nets are solved as a small DC
  circuit: the board's output pins and the batteries are sources,
  resistors, pots, LEDs (a forward voltage and a series resistance) and
  switches the elements, solved by nodal analysis over the few nets a
  hobby circuit has; the board's input pins read their net's voltage,
  its analogue pins its value. The lumped motor, driver and servo models
  stay as they are, fed by their nets instead of by name. Still no SPICE
  (route 3 of [ELECTRONICS.md](ELECTRONICS.md)): no transistor or
  capacitor dynamics.
- **The view.** A wiring sheet beside the 3D view (like a drawing's
  sheet): each part as its pin-out, placed by dragging; dragging from a
  pin to another draws a wire, routed square with corners you can move;
  colours by role (red supply, black ground, others free). Checks as it
  is drawn: an input left floating, two outputs shorted, a motor or a
  servo's supply on a board pin, an LED with no resistor, a supply over
  the board's 5 V.
- **Wokwi.** Reading a Wokwi project's `diagram.json` (its parts by type
  — `wokwi-arduino-uno`, `wokwi-led`, `wokwi-pushbutton`, `wokwi-servo`,
  `wokwi-potentiometer`, resistors — and its `connections`, each
  `["uno:13", "led1:A", colour, route]`) and its sketch, so a circuit
  tested in Wokwi comes into Forma wired; parts it doesn't know are
  listed and skipped.
- **Tests.** Netlists from wires (nets, a short found), the DC solve
  against hand-worked circuits (a divider, an LED with its resistor, a
  button pulling an input), the existing examples run unchanged from
  their converted wiring, a Wokwi project read.

## M22 cables

**What it gives.** The circuit's wires as cables in the assembly: from
the pin of one part's 3D model to the pin of another, along a path you
shape round the enclosure, as thick as their gauge and coloured as the
wire, checked for clashes and too-tight bends, their lengths in the parts
list and on drawings.

- **Where they go from.** The parts' 3D models get terminal points —
  header pins, a motor's tabs, a battery's leads, a servo's plug — in the
  library (`PartModels`), so a wire of M21 knows where it starts and ends
  in 3D once its parts are placed in the assembly.
- **The cable.** A Cable step per wire (or a bundle of several, a
  harness): from terminal to terminal through waypoints picked on faces
  or typed, a smooth curve through them, swept as a round section of the
  wire's diameter (AWG or mm²) into a body. The kernel has no swept
  surfaces of a path yet, so like the modelled threads it is a faceted
  solid — a tube of flat facets — unless a pipe surface comes first.
  It follows the parts when they move or a joint turns (its ends are
  jointed to the terminals, its waypoints to faces of the parts they
  were picked on).
- **Checks.** Clashes with the enclosure and the other parts (the
  existing interference check, the cable's own ends left out), a bend
  under the minimum radius for its gauge, a cable stretched taut between
  parts that move apart (its length against the path's).
- **The parts list and drawings.** Cut lengths with slack per wire and
  gauge in the bill of materials, a wire list (from, to, colour, gauge,
  length) as a CSV, and cables drawn in views like any body.
- **Tests.** Terminal points on the library's models, a cable's path and
  length, the tube a valid solid following its parts when they move, a
  clash and a tight bend found.

## Order

M21 first: it makes the wiring a thing in the design, which M22 then
draws in 3D. Each can ship on its own: M21 without cables still wires and
simulates a circuit; M22 could route cables from today's named wiring,
but needs M21's terminals to know which pin of a part a wire ends at.
