// A servo arm sweeping 20°..160° and back, and a wheel on a TT motor
// whose speed is the pot on A0 (a button on D2 reverses it); the LED on
// D13 blinks, and the sketch reports twice a second.
#include <Servo.h>

Servo arm;
const int IN1 = 7, IN2 = 8, EN = 6, LED = 13, BTN = 2;
int angle = 90, dir = 2;
unsigned long last = 0, lastPrint = 0;

void setup() {
  Serial.begin(115200);
  arm.attach(9);
  arm.write(angle);
  pinMode(IN1, OUTPUT);
  pinMode(IN2, OUTPUT);
  pinMode(LED, OUTPUT);
  pinMode(BTN, INPUT_PULLUP);
  Serial.println("sweep and drive");
}

void loop() {
  unsigned long now = millis();
  if (now - last >= 15) {
    last = now;
    angle += dir;
    if (angle >= 160 || angle <= 20) dir = -dir;
    arm.write(angle);
  }
  int speed = analogRead(A0) / 4;
  bool reverse = digitalRead(BTN) == LOW;
  digitalWrite(IN1, !reverse);
  digitalWrite(IN2, reverse);
  analogWrite(EN, speed);
  digitalWrite(LED, (now / 250) % 2);
  if (now - lastPrint >= 500) {
    lastPrint = now;
    Serial.print("arm ");
    Serial.print(angle);
    Serial.print(" wheel ");
    Serial.println(speed);
  }
}
