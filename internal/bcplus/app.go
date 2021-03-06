package bcplus

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"git.fractalqb.de/fractalqb/c4hgol"
	"git.fractalqb.de/fractalqb/qbsllm"
	"github.com/CmdrVasquess/bcplus/internal/common"
	"github.com/CmdrVasquess/bcplus/internal/wapp"
	"github.com/CmdrVasquess/goedx"
	"github.com/CmdrVasquess/goedx/apps/bboltgalaxy"
	"github.com/CmdrVasquess/goedx/apps/l10n"
	"github.com/CmdrVasquess/goedx/events"
	"github.com/CmdrVasquess/goedx/journal"
)

var (
	App    bcpApp
	LogWrs = []io.Writer{os.Stderr, &webLog}
	LogCfg = c4hgol.Config(qbsllm.NewConfig(log),
		goedx.LogCfg,
		l10n.LogCfg,
		wapp.LogCfg,
	)
)

var (
	log     = qbsllm.New(qbsllm.Lnormal, "BC+", nil, nil)
	edState = goedx.NewEDState()
)

const (
	l10nDir = "l10n"
)

type bcpApp struct {
	goedx.Extension
	WebPort    int
	dataDir    string
	assetDir   string
	webAddr    string
	webPin     string
	webTheme   string
	webTLS     bool
	debugModes string
	appL10n    *l10n.Locales
	evtq       chan interface{}
}

func ensureDatadir(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Infoa("create `data dir`", dir)
		if err = os.MkdirAll(dir, common.DirFileMode); err != nil {
			log.Fatale(err)
		}
	}
	l10n := filepath.Join(dir, l10nDir)
	if _, err := os.Stat(l10n); os.IsNotExist(err) {
		log.Infoa("create `localization dir`", l10n)
		if err = os.Mkdir(l10n, common.DirFileMode); err != nil {
			log.Fatale(err)
		}
	}
}

func (bcp *bcpApp) Init() {
	bcp.evtq = make(chan interface{}, 32)
	log.Infoa("data `dir`", bcp.dataDir)
	ensureDatadir(bcp.dataDir)
	log.Debuga("asset `dir`", bcp.assetDir)
	if bcp.webTLS {
		mustTLSCert(bcp.dataDir)
	}
	var err error
	if err = edState.Load(bcp.stateFile()); os.IsNotExist(err) {
		log.Infoa("state `file` not exists", bcp.stateFile())
	}
	bcp.EDState = edState
	if strings.IndexRune(App.debugModes, 'h') < 0 {
		bcp.JournalAfter = edState.LastJournalEvent
	}
	bcp.CmdrFile = func(cmdr *goedx.Commander) string {
		return filepath.Join(bcp.dataDir, cmdr.FID, "commander.json")
	}
	file := filepath.Join(bcp.dataDir, "galaxy.bbolt")
	bcp.Galaxy, err = bboltgalaxy.Open(file)
	if err != nil {
		log.Fatale(err)
	}
	bcp.AddApp("bcp", goedx.NewAppChannel(bcp, 16))
	file = filepath.Join(bcp.dataDir, l10nDir)
	bcp.appL10n = l10n.New(file, edState)
	bcp.AddApp("l10n", bcp.appL10n)
	initWebUI()
}

func (bcp *bcpApp) SaveState() {
	var file string
	if bcp.EDState.Cmdr != nil && bcp.EDState.Cmdr.FID != "" {
		file = bcp.CmdrFile(bcp.EDState.Cmdr)
	}
	err := edState.Save(bcp.stateFile(), file)
	if err != nil {
		log.Errore(err)
	}
}

func (bcp *bcpApp) Shutdown() {
	close(bcp.evtq)
	bcp.Extension.Stop()
	bcp.appL10n.Close()
	err := bcp.Galaxy.(*bboltgalaxy.Galaxy).Close()
	if err != nil {
		log.Errore(err)
	}
	bcp.SaveState()
}

func (bcp *bcpApp) PrepareEDEvent(e events.Event) (token interface{}) {
	return true
}

func (bcp *bcpApp) FinishEDEvent(_ interface{}, e events.Event, chg goedx.Change) {
	if _, ok := e.(*journal.Shutdown); ok {
		err := edState.Save(bcp.stateFile(), "")
		if err != nil {
			log.Errore(err)
		}
	}
	if chg == 0 || currentScreen == nil {
		return
	}
	const ScreenHdrChg = goedx.ChgCommander | goedx.ChgShip | goedx.ChgLocation |
		goedx.ChgSystem
	var msg []byte
	var err error
	data := wuiEvent{
		Cmd:    wuiCmdUpdate,
		Screen: currentScreen.Key,
	}
	err = bcp.EDState.Read(func() error {
		if chg.Any(ScreenHdrChg) {
			data.Hdr = wapp.NewScreenHdr(bcp.EDState)
		}
		if currentScreen != nil {
			data.Data = currentScreen.Handler.Data(chg)
		}
		msg, err = json.Marshal(&data)
		return err
	})
	if err != nil {
		log.Errore(err)
	} else {
		webApp.Write(msg)
	}
}

func (bcp *bcpApp) stateFile() string {
	return filepath.Join(bcp.dataDir, "bcplus.json")
}

func stdAssetDir() string {
	dir := filepath.Dir(os.Args[0])
	return filepath.Join(dir, "assets")
}

func (bcp *bcpApp) Flags() {
	jDir, err := goedx.FindJournals()
	if err != nil {
		jDir = ""
	}
	flag.StringVar(&App.JournalDir, "j", jDir, docJournalDir)
	flag.StringVar(&App.dataDir, "d", stdDataDir(), docDataDir)
	flag.StringVar(&App.assetDir, "assets", stdAssetDir(), docAssetDir)
	flag.IntVar(&App.WebPort, "web-port", 1337, docWebPort)
	flag.StringVar(&App.webAddr, "web-addr", "", docWebAddr)
	flag.StringVar(&App.webPin, "web-pin", "", docWebPin)
	flag.StringVar(&App.webTheme, "web-theme", "dark", docWebTheme)
	flag.BoolVar(&App.webTLS, "web-tls", true, docWebTLS)
	flag.StringVar(&App.debugModes, "debug", "", docDebug)
}

func (app *bcpApp) Run(signals <-chan os.Signal) {
	log.Infof("Running BC+ v%d.%d.%d-%s+%d (%s)",
		common.BCpMajor, common.BCpMinor, common.BCpPatch,
		common.BCpQuality, common.BCpBuildNo,
		runtime.Version())
	app.Init()
	go app.eventLoop()
	go app.Extension.MustRun(true)
	go runWebUI()
	<-signals
	log.Infof("BC+ %s interrupted; shutting down...", common.VersionLong)
	app.Shutdown()
}

func (app *bcpApp) eventLoop() {
	log.Infos("run event loop")
	for e := range app.evtq {
		switch evt := e.(type) {
		case *wsEvent:
			doWsEvent(evt)
		default:
			et := reflect.TypeOf(evt)
			if et.Kind() == reflect.Ptr {
				et = et.Elem()
			}
			log.Warna("unknown `event type`", et.Name())
		}
	}
	log.Infos("exit event loop")
}
