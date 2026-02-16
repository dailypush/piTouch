Make instructions

Yes, you can use Go (Golang) to control the Waveshare EPD_2in9_V2 e-Paper display on your Raspberry Pi. The periph library is a popular choice for interfacing with peripherals on the Raspberry Pi using Go. It has built-in support for e-Paper displays, including Waveshare e-Paper modules.

Here's how you can set up and use the periph library with the EPD_2in9_V2 display:

Install Go on your Raspberry Pi if you haven't already:
```bash
sudo apt-get install golang-go
```

Set up your Go workspace and environment variables:

```bash
mkdir ~/go
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc
```

Install the periph library:
```bash
go get -u periph.io/x/periph/cmd/...
```