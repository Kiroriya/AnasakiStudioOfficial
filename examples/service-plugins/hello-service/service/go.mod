module example.com/hello-service

go 1.22

require github.com/anasaki/studio/sdk/go v0.0.0

// Inside this repository; outside it, drop the replace and require a released SDK version.
replace github.com/anasaki/studio/sdk/go => ../../../../sdk/go
