package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/logutils"
	"github.com/jancona/m17"
	"gopkg.in/ini.v1"
	// _ "net/http/pprof"
)

type config struct {
	callsign         string
	dashboardLogger  *slog.Logger
	duplex           bool
	rxFrequency      uint32
	txFrequency      uint32
	power            float32
	afc              bool
	frequencyCorr    int16
	defaultReflector string
	defaultModule    string
	logLevel         string
	logPath          string
	logRoot          string
	modemType        string
	modemCfg         *ini.Section
	// modemPort        string
	// modemSpeed       int
	// nRSTPin          int
	// paEnablePin      int
	// boot0Pin         int
	symbolsIn        *os.File
	symbolsOut       *os.File
	hostfile         *m17.Hostfile
	overrideHostfile *m17.Hostfile
}

func loadConfig(iniFile string, inFile string, outFile string) (config, error) {
	log.Printf("[INFO] Loading settings from '%s'", iniFile)
	cfg, err := ini.Load(iniFile)
	if err != nil {
		log.Fatalf("Fail to read config from %s: %v", iniFile, err)
	}
	callsign := cfg.Section("General").Key("Callsign").String()
	dashboardLog := cfg.Section("General").Key("DashboardLog").String()
	rxFrequency, rxFrequencyErr := cfg.Section("Radio").Key("RXFrequency").Uint()
	txFrequency, txFrequencyErr := cfg.Section("Radio").Key("TXFrequency").Uint()
	power, powerErr := cfg.Section("Radio").Key("Power").Float64()
	afc, afcErr := cfg.Section("Radio").Key("AFC").Bool()
	frequencyCorr, frequencyCorrErr := cfg.Section("Radio").Key("FrequencyCorr").Int()
	duplex, duplexErr := cfg.Section("Radio").Key("Duplex").Bool()

	hostFile := cfg.Section("Reflector").Key("HostFile").String()
	overrideHostFile := cfg.Section("Reflector").Key("OverrideHostFile").String()
	reflectorName := cfg.Section("Reflector").Key("Name").String()
	reflectorModule := cfg.Section("Reflector").Key("Module").String()
	logLevel := cfg.Section("Log").Key("Level").String()
	logPath := cfg.Section("Log").Key("Path").String()
	logRoot := cfg.Section("Log").Key("Root").String()
	var modemType string
	var modemTypeErr error
	if !cfg.Section("Modem").HasKey("Type") {
		cfg.Section("Modem").Key("Type").SetValue("cc1200")
	}
	modemType = cfg.Section("Modem").Key("Type").In("BAD", []string{"cc1200", "mmdvm", "dummy"})
	if modemType == "BAD" {
		modemTypeErr = fmt.Errorf("bad Modem Type: %s", cfg.Section("Modem").Key("Type").String())
	}
	modemCfg := cfg.Section("Modem")

	_, callsignErr := m17.EncodeCallsign(callsign)
	// TODO: Lots of these validations are CC1200 specific
	if rxFrequencyErr == nil {
		if rxFrequency < 420e6 || rxFrequency > 450e6 {
			rxFrequencyErr = fmt.Errorf("configured RXFrequency %d out of range (420 to 450 MHz)", rxFrequency)
		}
	}
	if txFrequencyErr == nil {
		if txFrequency < 420e6 || txFrequency > 450e6 {
			txFrequencyErr = fmt.Errorf("configured TXFrequency %d out of range (420 to 450 MHz)", txFrequency)
		}
	}
	if powerErr == nil {
		if power < -16 || power > 14 {
			powerErr = fmt.Errorf("configured Power %f out of range (-16 to 14 dBm)", power)
		}
	}

	var reflectorHostfile, reflectorOverrideHostfile *m17.Hostfile
	var reflectorHostfileErr, reflectorOverrideHostfileErr error
	if hostFile != "" {
		reflectorHostfile, reflectorHostfileErr = m17.NewHostfile(hostFile)
	}
	if overrideHostFile != "" {
		reflectorOverrideHostfile, reflectorOverrideHostfileErr = m17.NewHostfile(overrideHostFile)
	}
	var reflectorModuleErr error
	if len(reflectorModule) > 1 {
		reflectorModuleErr = fmt.Errorf("configured Reflector Module must be zero or one character")
	}
	if reflectorModule == " " {
		reflectorModule = ""
	}
	var logLevelErr error
	if logLevel != "ERROR" && logLevel != "INFO" && logLevel != "DEBUG" {
		logLevelErr = fmt.Errorf("configured Log Level must be one of ERROR, INFO or DEBUG")
	}

	var symbolsInErr, symbolsOutErr error
	symbolsIn := os.Stdin
	if inFile != "" {
		symbolsIn, symbolsInErr = os.Open(inFile)
	}
	symbolsOut := os.Stdout
	if outFile != "" {
		symbolsOut, symbolsOutErr = os.Create(outFile)
	}

	var dashboardLogFile *os.File
	var dashboardLogErr error
	var dashboardLogger *slog.Logger
	if dashboardLog != "" {
		dashboardLogFile, dashboardLogErr = os.OpenFile(dashboardLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if dashboardLogFile != nil {
			opts := &slog.HandlerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.LevelKey || a.Key == slog.MessageKey {
						return slog.Attr{} // Remove the attribute
					}
					return a
				},
			}
			dashboardLogger = slog.New(slog.NewJSONHandler(dashboardLogFile, opts))
		}
	}

	err = errors.Join(
		rxFrequencyErr,
		txFrequencyErr,
		powerErr,
		afcErr,
		frequencyCorrErr,
		duplexErr,
		modemTypeErr,
		// modemSpeedErr,
		// nRSTPinErr,
		// paEnablePinErr,
		// boot0PinErr,
		callsignErr,
		reflectorModuleErr,
		logLevelErr,
		symbolsInErr,
		symbolsOutErr,
		dashboardLogErr,
		reflectorHostfileErr,
		reflectorOverrideHostfileErr,
	)

	return config{
		callsign:         callsign,
		duplex:           duplex,
		rxFrequency:      uint32(rxFrequency),
		txFrequency:      uint32(txFrequency),
		power:            float32(power),
		afc:              afc,
		frequencyCorr:    int16(frequencyCorr),
		defaultReflector: reflectorName,
		defaultModule:    reflectorModule,
		logLevel:         logLevel,
		logPath:          logPath,
		logRoot:          logRoot,
		modemType:        modemType,
		modemCfg:         modemCfg,
		// modemPort:        modemPort,
		// modemSpeed:       modemSpeed,
		// nRSTPin:          nRSTPin,
		// paEnablePin:      paEnablePin,
		// boot0Pin:         boot0Pin,
		symbolsIn:        symbolsIn,
		symbolsOut:       symbolsOut,
		dashboardLogger:  dashboardLogger,
		hostfile:         reflectorHostfile,
		overrideHostfile: reflectorOverrideHostfile,
	}, err
}

var (
	inArg      *string = flag.String("in", "", "M17 symbol input (default stdin)")
	outArg     *string = flag.String("out", "", "M17 symbol output (default stdout)")
	configFile *string = flag.String("config", "./gateway.ini", "Configuration file")
	reset      *bool   = flag.Bool("reset", false, "Reset modem and exit")
	helpArg    *bool   = flag.Bool("h", false, "Print arguments")
)

func main() {
	var err error

	flag.Parse()

	if *helpArg {
		flag.Usage()
		return
	}
	cfg, err := loadConfig(*configFile, *inArg, *outArg)
	if err != nil {
		log.Fatalf("Bad configuration: %v", err)
	}

	setupLogging(cfg)

	// // Server for pprof
	// go func() {
	// 	fmt.Println(http.ListenAndServe(":6060", nil))
	// }()

	var g *Gateway
	var modem m17.Modem
	switch cfg.modemType {
	case "cc1200":
		modem, err = m17.NewCC1200Modem(cfg.rxFrequency, cfg.txFrequency, cfg.power, cfg.frequencyCorr, cfg.afc, cfg.modemCfg)
		if err != nil {
			log.Fatalf("Error creating CC1200 modem: %v", err)
		}
		log.Printf("[INFO] Connected to CC1200 modem on %s", cfg.modemCfg.Key("Port").String())
	case "mmdvm":
		modem, err = m17.NewMMDVMModem(cfg.rxFrequency, cfg.txFrequency, cfg.power, cfg.frequencyCorr, cfg.afc, cfg.modemCfg, cfg.duplex)
		if err != nil {
			log.Fatalf("Error creating MMDVM modem: %v", err)
		}
		log.Printf("[INFO] Connected to MMDVM modem on %s", cfg.modemCfg.Key("Port").String())
	case "dummy":
		modem = &m17.DummyModem{
			In:  cfg.symbolsIn,
			Out: cfg.symbolsOut,
		}
	}

	if *reset {
		log.Print("[INFO] Resetting modem")
		err = modem.Reset()
		if err != nil {
			log.Printf("[ERROR] Error resetting modem: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	log.Printf("[DEBUG] Creating gateway cfg: %#v, modem %#v", cfg, modem)
	g, err = NewGateway(cfg, modem)
	if err != nil {
		log.Fatalf("Error creating Gateway: %v", err)
	}
	defer g.Close()
	g.Run()
}

func setupLogging(c config) {
	var err error
	minLogLevel := c.logLevel
	logWriter := os.Stderr

	if c.logRoot != "" {
		logWriter, err = os.OpenFile(c.logPath+"/"+c.logRoot+".log", os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0644)
		if err != nil {
			log.Fatalf("Error opening server output, exiting: %v", err)
		}
	}

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "ERROR"},
		MinLevel: logutils.LogLevel(minLogLevel),
		Writer:   logWriter,
	}
	log.SetOutput(filter)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	// log.SetFlags(0)
	log.Print("[DEBUG] Debug is on")
}

// Gateway connects to a reflector, converts traffic to/from audio format on stdout,
// so it can be used in a pipeline with other tools
type Gateway struct {
	Name   string
	Server string
	Port   uint
	Module string

	modem            m17.Modem
	in               *os.File
	out              *os.File
	relay            *m17.Relay
	duplex           bool
	done             bool
	dashboardLogger  *slog.Logger
	hostfile         *m17.Hostfile
	overrideHostfile *m17.Hostfile
}

func NewGateway(cfg config, modem m17.Modem) (*Gateway, error) {
	var err error

	g := Gateway{
		Name:             cfg.defaultReflector,
		Module:           cfg.defaultModule,
		modem:            modem,
		duplex:           cfg.duplex,
		dashboardLogger:  cfg.dashboardLogger,
		hostfile:         cfg.hostfile,
		overrideHostfile: cfg.overrideHostfile,
	}
	h, ok := g.overrideHostfile.Hosts[g.Name]
	if !ok {
		h, ok = g.hostfile.Hosts[g.Name]
		if !ok {
			return nil, fmt.Errorf("reflector %s not found", g.Name)
		}
	}
	g.Server = h.Server
	g.Port = h.Port
	log.Printf("[DEBUG] Connecting to %s, %s:%d, module %s", g.Name, g.Server, g.Port, g.Module)
	g.relay, err = m17.NewRelay(g.Name, g.Server, g.Port, g.Module, cfg.callsign, cfg.dashboardLogger, g.TransmitPacket, g.TransmitVoiceStream)
	if err != nil {
		return nil, fmt.Errorf("error creating relay: %v", err)
	}
	err = g.relay.Connect()
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s %s:%d %s: %v", g.Name, g.Server, g.Port, g.Module, err)
	}

	modem.Start()

	return &g, nil
}

func (g Gateway) TransmitPacket(p m17.Packet) error {
	// log.Printf("[DEBUG] received packet from relay: %#v", p)
	return g.modem.TransmitPacket(p)
}

func (g Gateway) TransmitVoiceStream(sd m17.StreamDatagram) error {
	// log.Printf("[DEBUG] received voice stream data from relay: %#v", sd)
	return g.modem.TransmitVoiceStream(sd)
}

func (g *Gateway) SendToNetwork(lsf *m17.LSF, payload []byte, sid, fn uint16) error {
	var err error
	if lsf == nil {
		return fmt.Errorf("nil lsf in SendToNetwork")
	}
	// log.Printf("[DEBUG] SendToNetwork lsf: %v, payload: % x, sid: %x, fn: %d", lsf, payload, sid, fn)
	if lsf.LSFType() == m17.LSFTypePacket {
		p := m17.NewPacketFromBytes(append(lsf.ToBytes(), payload...))
		log.Printf("[DEBUG] send packet to reflector/relay: %v", p)
		err = g.relay.SendPacket(p)
		if g.duplex {
			g.modem.TransmitPacket(p)
		}
	} else { // m17.LSFTypeStream
		if payload != nil {
			// if payload is nil, this is an LSF frame, so there's nothing to send to the reflector
			err = g.relay.SendStream(lsf, sid, fn, payload)
		}
		if g.duplex {
			sd := m17.NewStreamDatagram(sid, fn, lsf, payload)
			err2 := g.modem.TransmitVoiceStream(sd)
			err = errors.Join(err, err2)
		}
	}
	// TODO: Handle error?
	return err
}

func (g *Gateway) Run() {
	signalChan := make(chan os.Signal, 1)
	d := m17.NewDecoder(g.dashboardLogger, g.SendToNetwork, g.duplex)
	g.modem.StartDecoding(d.DecodeFrame)
	// Run until we're terminated then clean up
	log.Print("[DEBUG] client: Waiting for close signal")
	// wait for a close signal then clean up
	cleanupDone := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		log.Print("[DEBUG] client: Received an interrupt, stopping...")
		// Cleanup goes here
		close(cleanupDone)
	}()
	<-cleanupDone
}

func (g *Gateway) Close() {
	log.Print("[DEBUG] Gateway.Close()")
	g.done = true
	g.relay.Close()
	if g.modem != nil {
		g.modem.Close()
	}
	if g.in != os.Stdin {
		g.in.Close()
	}
	if g.out != os.Stdout {
		g.out.Close()
	}
}
