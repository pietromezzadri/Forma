# Electronics in Forma: what is possible (research note, Nov 2026)

*Built in M15 (route 2, with the parts library of route 1): see
[MILESTONES.md](MILESTONES.md) and the Electronics section of
[LIMITATIONS.md](LIMITATIONS.md). The note below is as it was written.*

The question: can Forma help build something like an RC car or a project
with an Arduino in it — not only the plastic parts, but the electronics
and how the whole thing behaves?

There are three different things "electrical simulation" can mean. They
differ a lot in value and cost.

## 1. The electronics as parts of the assembly (cheap, useful now)

Boards, motors, servos and batteries as components with their real
outlines, mounting holes, connectors and keep-out space, so the case, the
chassis and the mounts are designed around them and interference checks
catch a battery that doesn't fit.

- Forma already imports STEP solids and checks interference (M6, M7), so
  most of this is content: a small library of common parts (Arduino Uno /
  Nano, SG90 servo, TT gear motor, 18650 / 2S LiPo packs, an L298N or
  TB6612 driver board) as `.forma` components with joint points at their
  holes and shafts.
- A KiCad board comes in as STEP through KiCad's own export or the
  KiCad StepUp FreeCAD workbench ([kicad.org](https://kicad.org/external-tools/stepup/),
  [kicadStepUpMod](https://GitHub.com/easyw/kicadStepUpMod)). The older
  IDF exchange is still common but is being replaced by STEP and native
  links ([Autodesk on IDF](https://www.autodesk.com/products/fusion-360/blog/?p=17804),
  [Altium on MCAD exchange](https://www.altium.com/documentation/altium-developer-center/best-practices/mcad-integration)).
- Missing in Forma: STEP import of B-spline faces (E2) — many vendor
  models use them — and colours / names per part on import.

## 2. Behaviour: the real firmware driving the mechanism (the interesting one)

Run the actual Arduino sketch, compiled to a `.hex`, on an emulated
microcontroller inside Forma, wire its pins to simple models of a battery,
a motor driver, DC motors and servos, and let those drive the assembly's
joints: a servo turns the steering joint, a motor spins a wheel joint at
the speed its voltage and load give it. That is a motion study driven by
code — something neither Fusion nor Onshape do.

- **The CPU.** avr8js, the core of the Wokwi simulator, is a small MIT-
  licensed AVR (ATmega328P: Uno, Nano) emulator in TypeScript; it
  emulates the CPU and its peripherals (timers, GPIO, USART), and the
  board around it is left to the host
  ([avr8js](https://github.com/wokwi/avr8js),
  [how it works](https://blog.wokwi.com/avr8js-simulate-arduino-in-javascript/)).
  A C# port shows it ports well ([avr8sharp](https://github.com/PyMCU/avr8sharp)).
  I found no maintained Go AVR emulator; C ones exist (simavr, simulavr,
  [slimavr](https://github.com/thcopeland/slimavr)). A Go port of
  avr8js's CPU, timers and GPIO is a few thousand lines — feasible, pure
  Go, testable headlessly like the kernel.
- **The electrical side** doesn't need a circuit simulator for an RC
  car: lumped models are enough and run in real time — a battery
  (voltage, internal resistance), an H-bridge (PWM duty → average
  voltage), a DC motor (back-EMF and torque constants, winding
  resistance, friction, gear ratio), a servo (pulse width → target
  angle, slew rate, stall torque). This is what Wokwi does too: servos
  are built in, DC motors and the L298N come as community "custom chips"
  ([Wokwi hardware](https://docs.wokwi.com/getting-started/supported-hardware),
  [L298N chip](https://github.com/drf5n/Wokwi-Chip-L298N)).
- **Wiring** could be a small netlist in the design (pin → part
  terminal), or read from a Wokwi `diagram.json`, so a circuit wired and
  tested in Wokwi brings its wiring and its firmware into the 3D model.
- **What it needs from Forma first:** joints driven over time (a motion
  study with a clock, not one value per regeneration), joint limits
  (M13), and for a car that rolls on the floor, rigid-body dynamics and
  wheel contact — a big job of its own. A first version can stay
  kinematic: wheels spin, steering turns, the car's path follows from
  the wheel speeds (a bicycle model), no tyre physics.

## 3. Circuit-level simulation (SPICE) (large, best left to other tools)

Transistor- and op-amp-level behaviour: a regulator's ripple, a filter's
response. ngspice can be embedded as a shared library with a C API, and a
Go binding exists ([ngspice shared](https://ngspice.sourceforge.io/shared.html)).
But the value is in a schematic editor and component models, which KiCad
(ngspice built in) and LTspice already do well. For multi-domain physics
(motor, gearbox, vehicle) the established route is Modelica / FMI
co-simulation ([OpenModelica FMI](https://openmodelica.org/doc/OpenModelicaUsersGuide/1.19/fmitlm.html),
[FMI standard](https://fmi-standard.org/docs/main/)).

## Opinion

- Do **1** soon: it is mostly a parts library and better STEP import, and
  it is what makes an RC car chassis or an Arduino case fit first time.
- **2** is worth a milestone after stress analysis: an AVR core in Go,
  lumped motor / servo / battery models, a pin netlist, and a timed
  motion study driving joints. Start kinematic (a servo steering, motors
  spinning wheels, the car's path drawn); add dynamics later.
- Skip **3** inside Forma; at most read a KiCad netlist or a Wokwi
  diagram for the wiring.
