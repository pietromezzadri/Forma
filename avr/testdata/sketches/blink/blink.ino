// millis(), delay() and Serial: D13 toggled every 100 ms, the count printed.
unsigned long last = 0;
int count = 0;

void setup() {
  pinMode(13, OUTPUT);
  Serial.begin(9600);
  Serial.println("hello");
  delay(50);
  Serial.println(millis());
}

void loop() {
  if (millis() - last >= 100) {
    last += 100;
    count++;
    digitalWrite(13, count & 1);
    if (count == 10) {
      Serial.print("ten at ");
      Serial.println(micros());
    }
  }
}
