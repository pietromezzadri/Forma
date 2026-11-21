// analogWrite on D6 (timer 0, the millis() timer) as an H-bridge's enable,
// D7 / D8 its direction: half speed forward for a second, then reverse.
void setup() {
  pinMode(7, OUTPUT);
  pinMode(8, OUTPUT);
  digitalWrite(7, HIGH);
  digitalWrite(8, LOW);
  analogWrite(6, 128);
  delay(1000);
  digitalWrite(7, LOW);
  digitalWrite(8, HIGH);
  analogWrite(6, 255);
}

void loop() {}
