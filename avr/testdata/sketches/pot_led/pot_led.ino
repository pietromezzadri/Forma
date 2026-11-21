// analogRead on A0 mapped to analogWrite on D3 (timer 2) and D10
// (timer 1); a button on D2 (INPUT_PULLUP) turns D13 on while pressed.
void setup() {
  pinMode(2, INPUT_PULLUP);
  pinMode(13, OUTPUT);
}

void loop() {
  int v = analogRead(A0);
  analogWrite(3, v / 4);
  analogWrite(10, 255 - v / 4);
  digitalWrite(13, digitalRead(2) == LOW);
}
