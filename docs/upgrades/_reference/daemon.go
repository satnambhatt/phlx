//go:build ignore
// +build ignore

// Reference scaffold for the optional phalanx daemon.
// Not compiled into the main build. Copy into `cmd/phalanx-daemon/` and
// flesh out the endpoints when you decide to ship the daemon upgrade.
//
// Target: long-running process on 127.0.0.1:7777 that hosts the scan API
// and runs the file watcher. Hooks can `POST /scan` instead of running
// scanner.Scan() inline. On ECONNREFUSED they fall back to inline scan.
//
// See ../daemon.md for the design notes.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/satnambhatt/phlx/internal/db"
	"github.com/satnambhatt/phlx/internal/scanner"
)

const defaultPort = "7777"

var started = time.Now()

func main() {
	port := os.Getenv("PHALANX_PORT")
	if port == "" {
		port = defaultPort
	}

	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "[phalanx] db open failed:", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /scan", handleScan)
	mux.HandleFunc("GET /status", handleStatus)
	mux.HandleFunc("GET /history", handleHistory)
	mux.HandleFunc("POST /watch", handleWatch)
	mux.HandleFunc("GET /config", handleConfig)
	mux.HandleFunc("POST /allow", handleAllow)

	// TODO: spin up internal/watcher in a goroutine
	// watcher.Start(ctx, eventChan)

	addr := "127.0.0.1:" + port
	fmt.Printf("[phalanx] holding the line on %s (pid %d)\n", addr, os.Getpid())
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "[phalanx] daemon exit:", err)
		os.Exit(1)
	}
}

type scanReq struct {
	Pkg       string `json:"pkg"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	var req scanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if req.Pkg == "" {
		http.Error(w, "pkg required", http.StatusBadRequest)
		return
	}
	if req.Ecosystem == "" {
		req.Ecosystem = "npm"
	}
	res, err := scanner.Scan(scanner.Request{
		Pkg: req.Pkg, Version: req.Version, Ecosystem: req.Ecosystem,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	action := "allowed"
	if !res.Allowed {
		action = "blocked"
	} else if len(res.Policy.Warn) > 0 {
		action = "warned"
	}
	_ = db.RecordInstall(db.Install{
		Pkg: req.Pkg, Version: req.Version, Ecosystem: req.Ecosystem, Action: action,
	})

	writeJSON(w, http.StatusOK, res)
}

func handleStatus(w http.ResponseWriter, _ *http.Request) {
	s, _ := db.GetStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "running",
		"pid":    os.Getpid(),
		"uptime": time.Since(started).Seconds(),
		"stats":  s,
	})
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	rows, _ := db.History(limit)
	writeJSON(w, http.StatusOK, map[string]any{"installs": rows})
}

func handleWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	if err := db.AddWatched(body.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"watching": body.Path})
}

func handleConfig(w http.ResponseWriter, _ *http.Request) {
	cfg, _ := db.ListConfig()
	writeJSON(w, http.StatusOK, cfg)
}

type allowReq struct {
	Pkg       string `json:"pkg"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
	Reason    string `json:"reason"`
}

func handleAllow(w http.ResponseWriter, r *http.Request) {
	var body allowReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if body.Version == "" {
		body.Version = "*"
	}
	if body.Ecosystem == "" {
		body.Ecosystem = "npm"
	}
	reason := body.Reason
	if reason == "" {
		reason = "manual bypass"
	}
	key := fmt.Sprintf("allow:%s:%s:%s", body.Ecosystem, body.Pkg, body.Version)
	if err := db.SetConfig(key, reason); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"allowed": fmt.Sprintf("%s@%s", body.Pkg, body.Version),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
