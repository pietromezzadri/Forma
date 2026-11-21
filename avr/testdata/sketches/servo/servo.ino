// The Servo library on D9: 45 degrees for half a second, then 135.
#include <Servo.h>

Servo s;

void setup() {
  s.attach(9);
  s.write(45);
  delay(500);
  s.write(135);
}

void loop() {}
