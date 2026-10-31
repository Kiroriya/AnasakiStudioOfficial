# Countdown Timer

บล็อกนับถอยหลัง (วัน ชั่วโมง นาที วินาที) สู่วันเปิดตัว โปรโมชัน หรืออีเวนต์ — แสดงข้อความที่กำหนดเมื่อถึงเวลา

A designer block counting down days, hours, minutes and seconds to a target date, then showing a custom message.

วันเวลาเป้าหมายตั้งต้นเป็นค่าตัวอย่าง (2099-12-31T23:59) — ใส่วันจริงของคุณในช่อง "วันเวลาเป้าหมาย" (เวลาตามเครื่องผู้ชม) /
The default target (2099-12-31T23:59) is only a sample — enter your own date (interpreted in the visitor's local time).

## Changelog

- 1.0.1 — timers also start for blocks added after the page loaded (designer preview, client-side navigation) and restart when the target changes; the sample target no longer expires soon.
