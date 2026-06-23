package hub

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Hub manages family groups, devices, and their registered folders.
type Hub struct {
	db   *sql.DB
	addr string
}

func New(dbPath, addr string) (*Hub, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	h := &Hub{db: db, addr: addr}
	if err := h.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return h, nil
}

func (h *Hub) Close() error { return h.db.Close() }

func (h *Hub) migrate() error {
	_, err := h.db.Exec(`
		CREATE TABLE IF NOT EXISTS groups (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			invite     TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS devices (
			id             TEXT PRIMARY KEY,
			group_id       TEXT NOT NULL,
			name           TEXT NOT NULL,
			device_id      TEXT NOT NULL DEFAULT '',
			registered_at  TEXT NOT NULL,
			last_seen      TEXT NOT NULL,
			FOREIGN KEY (group_id) REFERENCES groups(id)
		);
		CREATE TABLE IF NOT EXISTS dev_folders (
			device_id  TEXT NOT NULL,
			folder_id  TEXT NOT NULL,
			label      TEXT NOT NULL,
			path       TEXT NOT NULL DEFAULT '',
			folder_type TEXT NOT NULL DEFAULT 'remoteaccess',
			PRIMARY KEY (device_id, folder_id),
			FOREIGN KEY (device_id) REFERENCES devices(id)
		);
	`)
	return err
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func newInvite() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)[:6]
}

// ── Handlers ──

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/")

	// POST /api/groups
	if r.Method == "POST" && path == "groups" {
		h.createGroup(w, r)
		return
	}

	// POST /api/groups/join  (join by invite code in body)
	if r.Method == "POST" && path == "groups/join" {
		h.joinGroup(w, r)
		return
	}

	// POST /api/groups/<id>/join
	if r.Method == "POST" && len(path) > 7 && strings.HasPrefix(path, "groups/") && strings.HasSuffix(path, "/join") {
		h.joinGroup(w, r)
		return
	}

	// GET /api/groups/<id>
	// GET /api/groups/<id>/devices
	if r.Method == "GET" && strings.HasPrefix(path, "groups/") {
		rest := path[7:]
		idx := strings.Index(rest, "/")
		if idx < 0 {
			h.getGroup(w, r, rest)
			return
		}
		gid := rest[:idx]
		sub := rest[idx+1:]
		if sub == "devices" {
			h.getGroupDevices(w, r, gid)
			return
		}
	}

	// POST /api/devices/register
	// POST /api/devices/folders
	// POST /api/devices/ping
	if r.Method == "POST" && strings.HasPrefix(path, "devices/") {
		rest := path[8:]
		switch rest {
		case "register":
			h.registerDevice(w, r)
			return
		case "folders":
			h.registerFolders(w, r)
			return
		case "identity":
			h.updateIdentity(w, r)
			return
		case "ping":
			h.pingDevice(w, r)
			return
		}
	}

	// GET /api/devices/<id>/folders
	// DELETE /api/devices/<id>
	if strings.HasPrefix(path, "devices/") {
		rest := path[8:]
		idx := strings.Index(rest, "/")
		if idx < 0 {
			if r.Method == "DELETE" {
				h.deleteDevice(w, r, rest)
				return
			}
		} else {
			devID := rest[:idx]
			sub := rest[idx+1:]
			if r.Method == "GET" && sub == "folders" {
				h.getDeviceFolders(w, r, devID)
				return
			}
		}
	}

	http.Error(w, "not found", http.StatusNotFound)
}

// ── Group APIs ──

func (h *Hub) createGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	id := newID()
	invite := newInvite()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.db.Exec("INSERT INTO groups(id,name,invite,created_at) VALUES(?,?,?,?)",
		id, req.Name, invite, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"id": id, "name": req.Name, "invite": invite})
}

func (h *Hub) joinGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Invite    string `json:"invite"`
		DeviceName string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Invite == "" || req.DeviceName == "" {
		http.Error(w, "invite and device_name required", http.StatusBadRequest)
		return
	}
	var groupID, groupName string
	if err := h.db.QueryRow("SELECT id,name FROM groups WHERE invite=?", req.Invite).Scan(&groupID, &groupName); err != nil {
		http.Error(w, "invalid invite code", http.StatusNotFound)
		return
	}
	devID := newID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.db.Exec("INSERT INTO devices(id,group_id,name,registered_at,last_seen) VALUES(?,?,?,?,?)",
		devID, groupID, req.DeviceName, now, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{
		"device_id": devID, "group_id": groupID, "group_name": groupName, "device_name": req.DeviceName,
	})
}

func (h *Hub) getGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	var id, name, invite, createdAt string
	err := h.db.QueryRow("SELECT id,name,invite,created_at FROM groups WHERE id=?", groupID).
		Scan(&id, &name, &invite, &createdAt)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"id": id, "name": name, "invite": invite, "created_at": createdAt})
}

// ── Device APIs ──

func (h *Hub) getGroupDevices(w http.ResponseWriter, r *http.Request, groupID string) {
	rows, err := h.db.Query("SELECT id,name,device_id,registered_at,last_seen FROM devices WHERE group_id=?", groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var devices []map[string]string
	for rows.Next() {
		var id, name, devID, regAt, lastSeen string
		rows.Scan(&id, &name, &devID, &regAt, &lastSeen)
		devices = append(devices, map[string]string{
			"id": id, "name": name, "device_id": devID, "registered_at": regAt, "last_seen": lastSeen,
		})
	}
	writeJSON(w, devices)
}

func (h *Hub) registerDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID  string `json:"group_id"`
		Name     string `json:"name"`
		DeviceID string `json:"device_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.GroupID == "" || req.Name == "" {
		http.Error(w, "group_id and name required", http.StatusBadRequest)
		return
	}
	devID := newID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.db.Exec("INSERT INTO devices(id,group_id,name,device_id,registered_at,last_seen) VALUES(?,?,?,?,?,?)",
		devID, req.GroupID, req.Name, req.DeviceID, now, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"id": devID, "name": req.Name})
}

func (h *Hub) registerFolders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID string `json:"device_id"`
		Folders  []struct {
			FolderID   string `json:"folder_id"`
			Label      string `json:"label"`
			Path       string `json:"path"`
			FolderType string `json:"folder_type"`
		} `json:"folders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" || len(req.Folders) == 0 {
		http.Error(w, "device_id and folders required", http.StatusBadRequest)
		return
	}
	// Delete old registrations
	h.db.Exec("DELETE FROM dev_folders WHERE device_id=?", req.DeviceID)
	for _, f := range req.Folders {
		ft := f.FolderType
		if ft == "" {
			ft = "remoteaccess"
		}
		h.db.Exec("INSERT INTO dev_folders(device_id,folder_id,label,path,folder_type) VALUES(?,?,?,?,?)",
			req.DeviceID, f.FolderID, f.Label, f.Path, ft)
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Hub) getDeviceFolders(w http.ResponseWriter, r *http.Request, deviceID string) {
	rows, err := h.db.Query("SELECT folder_id,label,path,folder_type FROM dev_folders WHERE device_id=?", deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var folders []map[string]string
	for rows.Next() {
		var fid, label, path, ftype string
		rows.Scan(&fid, &label, &path, &ftype)
		folders = append(folders, map[string]string{
			"folder_id": fid, "label": label, "path": path, "folder_type": ftype,
		})
	}
	writeJSON(w, folders)
}

// updateIdentity records a device's syncthing device ID so peers can pair.
func (h *Hub) updateIdentity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID    string `json:"device_id"`    // hub device id
		SyncthingID string `json:"syncthing_id"` // syncthing device id
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" || req.SyncthingID == "" {
		http.Error(w, "device_id and syncthing_id required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.db.Exec("UPDATE devices SET device_id=?, last_seen=? WHERE id=?", req.SyncthingID, now, req.DeviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Hub) pingDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID string `json:"device_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.DeviceID != "" {
		h.db.Exec("UPDATE devices SET last_seen=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), req.DeviceID)
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Hub) deleteDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	h.db.Exec("DELETE FROM dev_folders WHERE device_id=?", deviceID)
	result, _ := h.db.Exec("DELETE FROM devices WHERE id=?", deviceID)
	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// ── Start ──

func (h *Hub) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.Handle("/api/", h)

	// Serve ShareBox frontend from local filesystem
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, "gui/sharebox/index.html")
			return
		}
		http.NotFound(w, r)
	})

	slog.Info("Hub listening", "addr", h.addr)
	return http.ListenAndServe(h.addr, mux)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
