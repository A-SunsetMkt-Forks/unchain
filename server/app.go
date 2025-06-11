package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unchainese/unchain/global"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

type App struct {
	cfg               *global.Config
	userUsedTrafficKb sync.Map // string -> int64
	svr               *http.Server
	exitSignal        chan os.Signal
}

func (app *App) httpSvr() {
	mux := http.NewServeMux()
	mux.HandleFunc("/wsv/{uid}", app.WsVLESS)
	mux.HandleFunc("/sub/{uid}", app.Sub)
	mux.HandleFunc("/ws-vless", app.WsVLESS)
	mux.HandleFunc("/", app.Ping)
	server := &http.Server{
		Addr:    app.cfg.ListenAddr(),
		Handler: mux,
	}
	app.svr = server

}

func NewApp(c *global.Config, sig chan os.Signal) *App {
	app := &App{
		cfg:               c,
		userUsedTrafficKb: sync.Map{},
		exitSignal:        sig,
		svr:               nil,
	}
	for _, userID := range c.UserIDS() {
		app.userUsedTrafficKb.Store(userID, int64(0))
	}
	app.httpSvr()
	go app.loopPush()
	return app
}

func (app *App) Run() {
	log.Println("server starting on http://", app.cfg.ListenAddr())
	if err := app.svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Could not listen on %s: %v\n", app.cfg.ListenAddr(), err)
	}
}

func (app *App) PrintVLESSConnectionURLS() {
	listenPort := app.cfg.ListenPort()

	fmt.Printf("\n\n\nvist to get VLESS connection info: http://127.0.0.1:%d/sub/<YOUR_CONFIGED_UUID> \n", listenPort)
	fmt.Printf("vist to get VLESS connection info: http://<HOST>:%d/sub/<YOUR_UUID>\n", listenPort)

	app.userUsedTrafficKb.Range(func(id, _ interface{}) bool {
		userID := id.(string)
		fmt.Println("\n------------- USER UUID:  ", userID, " -------------")
		urls := app.vlessUrls(userID)
		for _, url := range urls {
			fmt.Println(url)
		}
		return true
	})
	fmt.Println("\n\n\n")
}

func (app *App) Shutdown(ctx context.Context) {
	log.Println("Shutting down the server...")
	if err := app.svr.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exiting")
}

func (app *App) loopPush() {
	url := app.cfg.RegisterUrl
	if url == "" {
		log.Println("Register url is empty, skip register, runs in standalone mode")
		return
	}
	tk := time.NewTicker(app.cfg.PushInterval())
	defer tk.Stop()
	for {
		select {
		case sig := <-app.exitSignal:
			app.exitSignal <- sig
			app.PushNode() //last push
			return
		case <-tk.C:
			app.PushNode()
		}
	}
}

func (app *App) trafficInc(uid string, byteN int64) {
	if app.cfg.DisableUserTraffic() {
		return
	}
	kb := byteN >> 10
	value, ok := app.userUsedTrafficKb.Load(uid)
	if ok {
		iv, isInt64 := value.(int64)
		if isInt64 {
			kb += iv
		} else {
			slog.Error("not a int64", "uid", uid, "value", value)
		}
	}
	app.userUsedTrafficKb.Store(uid, kb)
}

func (app *App) stat() *AppStat {
	data := make(map[string]int64)
	app.userUsedTrafficKb.Range(func(key, value interface{}) bool {
		data[key.(string)] = value.(int64)
		return true
	})
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
		slog.Error(err.Error())
	}
	res := &AppStat{
		Traffic:     data,
		Hostname:    hostname,
		Goroutine:   int64(runtime.NumGoroutine()),
		VersionInfo: app.cfg.GitHash + " -> " + app.cfg.BuildTime,
	}
	res.SubAddresses = app.cfg.SubHostWithPort()
	return res
}

type AppStat struct {
	Traffic      map[string]int64 `json:"traffic"`
	Hostname     string           `json:"hostname"`
	SubAddresses []string         `json:"sub_addresses"`
	Goroutine    int64            `json:"goroutine"`
	VersionInfo  string           `json:"version_info"`
}

func (app *App) PushNode() {
	url := app.cfg.RegisterUrl
	if url == "" {
		return
	}
	args := app.stat()
	body := bytes.NewBuffer(nil)
	err := json.NewEncoder(body).Encode(args)
	if err != nil {
		log.Println("Error encoding request:", err)
		return
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		log.Println("Error registering:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", app.cfg.RegisterToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error registering:", err)
		return
	}
	defer resp.Body.Close()
	users := make(map[string]int64)
	err = json.NewDecoder(resp.Body).Decode(&users)
	if err != nil {
		log.Println("Error decoding response:", err)
		return
	}
	//app.userUsedTrafficKb.Clear()
	//above will remove the standalone instance's UUID
	for k, userAvailableKB := range users {
		slog.Debug("user available traffic", "uid", k, "available", userAvailableKB)
		app.userUsedTrafficKb.Store(k, int64(0)) //set allowed userID
	}
}

func (app *App) IsUserNotAllowed(uuid string) (isNotAllowed bool) {
	_, ok := app.userUsedTrafficKb.Load(uuid)
	return !ok
}
