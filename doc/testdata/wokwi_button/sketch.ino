// A button on D2 lights the LED on D13; the pot on A0 sets the servo on D9.
#include <Servo.h>

Servo arm;

void setup() {
  pinMode(2, INPUT_PULLUP);
  pinMode(13, OUTPUT);
  arm.attach(9);
}

void loop() {
  digitalWrite(13, digitalRead(2) == LOW ? HIGH : LOW);
  arm.write(map(analogRead(A0), 0, 1023, 0, 180));
  delay(15);
}
