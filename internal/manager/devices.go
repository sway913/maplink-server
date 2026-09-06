package manager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	deviceRegistryVersion = 1
	devicePersistInterval = time.Minute
	pairingCodeTTL        = 10 * time.Minute
	maxPendingPairings    = 16
)

type deviceRecord struct {
	DeviceID   string     `json:"deviceID"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	Credential string     `json:"credential"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeen   time.Time  `json:"lastSeen"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type deviceRegistryFile struct {
	Version               int            `json:"version"`
	AllowLegacyRemoteAuth *bool          `json:"allowLegacyRemoteAuth,omitempty"`
	Devices               []deviceRecord `json:"devices"`
}

type deviceRegistry struct {
	mu          sync.Mutex
	path        string
	devices     map[string]deviceRecord
	lastPersist map[string]time.Time
	allowLegacy bool
}

type pendingEnrollment struct {
	Key       [sha256.Size]byte
	ExpiresAt time.Time
}

type deviceView struct {
	DeviceID   string    `json:"deviceID"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeen   time.Time `json:"lastSeen"`
	Revoked    bool      `json:"revoked"`
	Online     bool      `json:"online"`
	Permission string    `json:"permission,omitempty"`
	Legacy     bool      `json:"legacy"`
}

func loadDeviceRegistry(path string) (*deviceRegistry, error) {
	registry := &deviceRegistry{
		path:        path,
		devices:     make(map[string]deviceRecord),
		lastPersist: make(map[string]time.Time),
		allowLegacy: true,
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取设备注册表失败: %w", err)
	}
	var stored deviceRegistryFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("解析设备注册表失败: %w", err)
	}
	if stored.Version != deviceRegistryVersion {
		return nil, fmt.Errorf("不支持的设备注册表版本: %d", stored.Version)
	}
	if stored.AllowLegacyRemoteAuth != nil {
		registry.allowLegacy = *stored.AllowLegacyRemoteAuth
	}
	for _, device := range stored.Devices {
		if !validRemoteDeviceID(device.DeviceID) || device.Credential == "" {
			return nil, errors.New("设备注册表包含无效记录")
		}
		registry.devices[device.DeviceID] = device
		registry.lastPersist[device.DeviceID] = device.LastSeen
	}
	return registry, nil
}

func (r *deviceRegistry) persistLocked() error {
	devices := make([]deviceRecord, 0, len(r.devices))
	for _, device := range r.devices {
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].DeviceID < devices[j].DeviceID })
	allowLegacy := r.allowLegacy
	data, err := json.MarshalIndent(deviceRegistryFile{Version: deviceRegistryVersion, AllowLegacyRemoteAuth: &allowLegacy, Devices: devices}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeSecure(r.path, data, 0o600, 0)
}

func (r *deviceRegistry) legacyRemoteAuthAllowed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.allowLegacy
}

func (r *deviceRegistry) setLegacyRemoteAuthAllowed(allowed bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.allowLegacy
	r.allowLegacy = allowed
	if err := r.persistLocked(); err != nil {
		r.allowLegacy = previous
		return err
	}
	return nil
}

func (r *deviceRegistry) enroll(deviceID, name, platform, credential string, now time.Time) (deviceRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous, existed := r.devices[deviceID]
	createdAt := now
	if existed && !previous.CreatedAt.IsZero() {
		createdAt = previous.CreatedAt
	}
	record := deviceRecord{
		DeviceID: deviceID, Name: name, Platform: platform, Credential: credential,
		CreatedAt: createdAt, LastSeen: now,
	}
	r.devices[deviceID] = record
	if err := r.persistLocked(); err != nil {
		if existed {
			r.devices[deviceID] = previous
		} else {
			delete(r.devices, deviceID)
		}
		return deviceRecord{}, err
	}
	r.lastPersist[deviceID] = now
	return record, nil
}

func (r *deviceRegistry) credential(deviceID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceID]
	return device.Credential, ok && device.RevokedAt == nil
}

func (r *deviceRegistry) touch(deviceID, platform string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceID]
	if !ok || device.RevokedAt != nil {
		return errors.New("设备未注册或已撤销")
	}
	previous := device
	device.LastSeen = now
	if platform != "" {
		device.Platform = platform
	}
	r.devices[deviceID] = device
	if last := r.lastPersist[deviceID]; !last.IsZero() && now.Sub(last) < devicePersistInterval && previous.Platform == device.Platform {
		return nil
	}
	if err := r.persistLocked(); err != nil {
		r.devices[deviceID] = previous
		return err
	}
	r.lastPersist[deviceID] = now
	return nil
}

func (r *deviceRegistry) rename(deviceID, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceID]
	if !ok {
		return os.ErrNotExist
	}
	previous := device
	device.Name = name
	r.devices[deviceID] = device
	if err := r.persistLocked(); err != nil {
		r.devices[deviceID] = previous
		return err
	}
	return nil
}

func (r *deviceRegistry) revoke(deviceID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceID]
	if !ok {
		return os.ErrNotExist
	}
	previous := device
	device.RevokedAt = &now
	r.devices[deviceID] = device
	if err := r.persistLocked(); err != nil {
		r.devices[deviceID] = previous
		return err
	}
	return nil
}

func (r *deviceRegistry) list() []deviceRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	devices := make([]deviceRecord, 0, len(r.devices))
	for _, device := range r.devices {
		device.Credential = ""
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Name == devices[j].Name {
			return devices[i].DeviceID < devices[j].DeviceID
		}
		return devices[i].Name < devices[j].Name
	})
	return devices
}

func normalizePairingCode(value string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(value)))
}

func pairingCodeKey(value string) [sha256.Size]byte {
	return sha256.Sum256([]byte(normalizePairingCode(value)))
}

func pairingCodeID(value string) string {
	normalized := normalizePairingCode(value)
	if len(normalized) < 5 {
		return ""
	}
	return normalized[:5]
}

func newPairingCode() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	plain := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	return strings.Join([]string{plain[:5], plain[5:10], plain[10:15], plain[15:]}, "-"), nil
}

func (s *Server) createDeviceEnrollment(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC()
	expiresAt := now.Add(pairingCodeTTL)
	s.enrollmentsMu.Lock()
	for id, enrollment := range s.enrollments {
		if !enrollment.ExpiresAt.After(now) {
			delete(s.enrollments, id)
		}
	}
	if len(s.enrollments) >= maxPendingPairings {
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusTooManyRequests, errors.New("待使用配对码过多，请等待旧配对码过期"))
		return
	}
	var code string
	for attempts := 0; attempts < 4; attempts++ {
		candidate, err := newPairingCode()
		if err != nil {
			s.enrollmentsMu.Unlock()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if _, exists := s.enrollments[pairingCodeID(candidate)]; !exists {
			code = candidate
			break
		}
	}
	if code == "" {
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusInternalServerError, errors.New("生成唯一配对码失败"))
		return
	}
	s.enrollments[pairingCodeID(code)] = pendingEnrollment{Key: pairingCodeKey(code), ExpiresAt: expiresAt}
	s.enrollmentsMu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"code": code, "expiresAt": expiresAt})
}

func enrollmentProof(key [sha256.Size]byte, deviceID, name, platform, nonce string) []byte {
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(deviceID + "\n" + name + "\n" + platform + "\n" + nonce))
	return mac.Sum(nil)
}

func encryptEnrollmentResponse(key [sha256.Size]byte, value any) (map[string]string, error) {
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	var aead cipher.AEAD
	aead, err = cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, plain, []byte("maplink-device-enrollment-v1"))
	return map[string]string{
		"nonce":      base64.RawURLEncoding.EncodeToString(nonce),
		"ciphertext": base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func validDevicePlatform(value string) bool {
	return value == "windows" || value == "macos"
}

func (s *Server) clientEnroll(w http.ResponseWriter, r *http.Request) {
	var request struct {
		CodeID   string `json:"codeID"`
		Nonce    string `json:"nonce"`
		Proof    string `json:"proof"`
		DeviceID string `json:"deviceID"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	request.Name = strings.TrimSpace(request.Name)
	request.Platform = strings.ToLower(strings.TrimSpace(request.Platform))
	request.CodeID = normalizePairingCode(request.CodeID)
	providedProof, proofErr := hex.DecodeString(request.Proof)
	if len(request.CodeID) != 5 || len(request.Nonce) < 16 || len(request.Nonce) > 96 || proofErr != nil || len(providedProof) != sha256.Size ||
		!validDeviceID(request.DeviceID) || len(request.Name) > 128 || !validDevicePlatform(request.Platform) {
		writeError(w, http.StatusBadRequest, errors.New("设备配对信息无效"))
		return
	}
	if request.Name == "" {
		request.Name = request.DeviceID
	}
	settings, err := s.options.Store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	now := time.Now().UTC()
	s.enrollmentsMu.Lock()
	enrollment, exists := s.enrollments[request.CodeID]
	if !exists || !enrollment.ExpiresAt.After(now) {
		delete(s.enrollments, request.CodeID)
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusUnauthorized, errors.New("配对码无效或已过期"))
		return
	}
	expectedProof := enrollmentProof(enrollment.Key, request.DeviceID, request.Name, request.Platform, request.Nonce)
	if subtle.ConstantTimeCompare(providedProof, expectedProof) != 1 {
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusUnauthorized, errors.New("配对码无效或已过期"))
		return
	}
	credential, err := randomToken(32)
	if err != nil {
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	plainResponse := map[string]any{
		"deviceID": request.DeviceID, "deviceCredential": credential,
		"serverAddr": s.publicIP(), "serverPort": settings.BindPort, "managerPort": s.options.ManagerPort,
		"controlPorts": settings.ExpandedControlPorts(), "token": settings.Token, "protocol": "tcp",
	}
	encryptedResponse, err := encryptEnrollmentResponse(enrollment.Key, plainResponse)
	if err != nil {
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.devices.enroll(request.DeviceID, request.Name, request.Platform, credential, now); err != nil {
		s.enrollmentsMu.Unlock()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存设备失败: %w", err))
		return
	}
	delete(s.enrollments, request.CodeID)
	s.enrollmentsMu.Unlock()

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, encryptedResponse)
}

func (s *Server) deviceList(w http.ResponseWriter, _ *http.Request) {
	records := s.devices.list()
	views := make(map[string]deviceView, len(records))
	for _, device := range records {
		views[device.DeviceID] = deviceView{
			DeviceID: device.DeviceID, Name: device.Name, Platform: device.Platform,
			CreatedAt: device.CreatedAt, LastSeen: device.LastSeen, Revoked: device.RevokedAt != nil,
		}
	}
	now := time.Now()
	s.remote.mu.Lock()
	s.remote.cleanupLocked(now)
	for _, host := range s.remote.hosts {
		view, registered := views[host.DeviceID]
		if !registered {
			view = deviceView{DeviceID: host.DeviceID, Name: host.Name, Platform: host.Platform, Legacy: true}
		}
		if !view.Revoked {
			view.Online = true
			view.Permission = host.Permission
			view.LastSeen = host.LastSeen
		}
		views[host.DeviceID] = view
	}
	s.remote.mu.Unlock()
	result := make([]deviceView, 0, len(views))
	for _, view := range views {
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Online != result[j].Online {
			return result[i].Online
		}
		if result[i].Name == result[j].Name {
			return result[i].DeviceID < result[j].DeviceID
		}
		return result[i].Name < result[j].Name
	})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": result, "allowLegacyRemoteAuth": s.devices.legacyRemoteAuthAllowed(),
	})
}

func (s *Server) updateDevicePolicy(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AllowLegacyRemoteAuth *bool `json:"allowLegacyRemoteAuth"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.AllowLegacyRemoteAuth == nil {
		writeError(w, http.StatusBadRequest, errors.New("缺少旧版远控兼容设置"))
		return
	}
	if err := s.devices.setLegacyRemoteAuthAllowed(*request.AllowLegacyRemoteAuth); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存设备策略失败: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"allowLegacyRemoteAuth": *request.AllowLegacyRemoteAuth})
}

func (s *Server) renameDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	if !validDeviceID(deviceID) {
		writeError(w, http.StatusBadRequest, errors.New("设备 ID 无效"))
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 128 {
		writeError(w, http.StatusBadRequest, errors.New("设备名称长度必须为 1-128"))
		return
	}
	if err := s.devices.rename(deviceID, request.Name); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, errors.New("设备不存在"))
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	if !validDeviceID(deviceID) {
		writeError(w, http.StatusBadRequest, errors.New("设备 ID 无效"))
		return
	}
	if err := s.devices.revoke(deviceID, time.Now().UTC()); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, errors.New("设备不存在"))
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.remote.removeDevice(deviceID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *remoteHub) removeDevice(deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.hosts, deviceID)
	for _, session := range h.sessions {
		if session.TargetDeviceID == deviceID || session.ControllerDeviceID == deviceID {
			session.State = "closed"
			session.Frame = nil
			session.Inputs = nil
			session.UpdatedAt = time.Now()
		}
	}
}
