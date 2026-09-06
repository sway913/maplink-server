package manager

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testAdminSession struct {
	Cookie *http.Cookie
	CSRF   string
}

func loginAdmin(t *testing.T, server *Server) testAdminSession {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"a-strong-admin-password"}`))
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
		t.Fatalf("admin login failed: %d %s", response.Code, response.Body.String())
	}
	var body struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.CSRF == "" {
		t.Fatalf("invalid login response: %v %s", err, response.Body.String())
	}
	return testAdminSession{Cookie: response.Result().Cookies()[0], CSRF: body.CSRF}
}

func adminRequest(t *testing.T, server *Server, session testAdminSession, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.AddCookie(session.Cookie)
	if method != http.MethodGet {
		request.Header.Set("X-CSRF-Token", session.CSRF)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func createPairingCode(t *testing.T, server *Server, session testAdminSession) string {
	t.Helper()
	response := adminRequest(t, server, session, http.MethodPost, "/api/devices/enrollments", nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("create pairing code failed: %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(normalizePairingCode(body.Code)) != 20 {
		t.Fatalf("invalid pairing code response: %v %s", err, response.Body.String())
	}
	return body.Code
}

func enrollTestDevice(t *testing.T, server *Server, code, deviceID, platform string) string {
	t.Helper()
	name := "Test " + deviceID
	nonce := "test-enrollment-nonce-" + deviceID
	key := pairingCodeKey(code)
	body, err := json.Marshal(map[string]string{
		"codeID": pairingCodeID(code), "nonce": nonce, "proof": hex.EncodeToString(enrollmentProof(key, deviceID, name, platform, nonce)),
		"deviceID": deviceID, "name": name, "platform": platform,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/client/enroll", bytes.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("device enrollment failed: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Nonce      string `json:"nonce"`
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid enrollment envelope: %v %s", err, response.Body.String())
	}
	plain := decryptEnrollmentResponse(t, key, envelope.Nonce, envelope.Ciphertext)
	var result struct {
		Credential string `json:"deviceCredential"`
		Token      string `json:"token"`
	}
	if err := json.Unmarshal(plain, &result); err != nil || len(result.Credential) < 32 || result.Token == "" {
		t.Fatalf("invalid enrollment response: %v %s", err, response.Body.String())
	}
	return result.Credential
}

func decryptEnrollmentResponse(t *testing.T, key [32]byte, nonceValue, ciphertextValue string) []byte {
	t.Helper()
	nonce, err := base64.RawURLEncoding.DecodeString(nonceValue)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(ciphertextValue)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	var aead cipher.AEAD
	aead, err = cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := aead.Open(nil, nonce, ciphertext, []byte("maplink-device-enrollment-v1"))
	if err != nil {
		t.Fatalf("decrypt enrollment response: %v", err)
	}
	return plain
}

func pairingRequestBody(t *testing.T, code, deviceID, name, platform, nonce string) []byte {
	t.Helper()
	key := pairingCodeKey(code)
	body, err := json.Marshal(map[string]string{
		"codeID": pairingCodeID(code), "nonce": nonce, "proof": hex.EncodeToString(enrollmentProof(key, deviceID, name, platform, nonce)),
		"deviceID": deviceID, "name": name, "platform": platform,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func deviceRemoteRequest(t *testing.T, server *Server, deviceID, credential, method, path string, body []byte, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("X-MapLink-Device-ID", deviceID)
	request.Header.Set("X-MapLink-Timestamp", timestamp)
	request.Header.Set("X-MapLink-Nonce", nonce)
	request.Header.Set("X-MapLink-Signature", remoteSignature(credential, method, path, timestamp, nonce, body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func legacyRemoteRequest(t *testing.T, server *Server, credential, method, path string, body []byte, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("X-MapLink-Timestamp", timestamp)
	request.Header.Set("X-MapLink-Nonce", nonce)
	request.Header.Set("X-MapLink-Signature", remoteSignature(credential, method, path, timestamp, nonce, body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func TestDevicePairingIsSingleUsePersistentAndDoesNotExposeCredentialToAdmin(t *testing.T) {
	server := integrationServer(t)
	admin := loginAdmin(t, server)
	code := createPairingCode(t, server, admin)
	credential := enrollTestDevice(t, server, code, "office-pc", "windows")

	replayedBody := pairingRequestBody(t, code, "other-pc", "Other", "windows", "test-enrollment-replay-0001")
	replayed := httptest.NewRecorder()
	server.Handler().ServeHTTP(replayed, httptest.NewRequest(http.MethodPost, "/api/client/enroll", bytes.NewReader(replayedBody)))
	if replayed.Code != http.StatusUnauthorized {
		t.Fatalf("expected pairing code replay to fail, got %d", replayed.Code)
	}

	list := adminRequest(t, server, admin, http.MethodGet, "/api/devices", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "office-pc") || strings.Contains(list.Body.String(), credential) {
		t.Fatalf("unsafe or incomplete device list: %d %s", list.Code, list.Body.String())
	}
	info, err := os.Stat(server.options.DevicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("device registry permissions too broad: %o", info.Mode().Perm())
	}
	reloaded, err := loadDeviceRegistry(server.options.DevicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored, ok := reloaded.credential("office-pc"); !ok || stored != credential {
		t.Fatal("device credential did not persist")
	}
}

func TestDeviceCredentialBindsIdentityAndRevocationClosesAccess(t *testing.T) {
	server := integrationServer(t)
	admin := loginAdmin(t, server)
	targetCredential := enrollTestDevice(t, server, createPairingCode(t, server, admin), "office-pc", "windows")
	controllerCredential := enrollTestDevice(t, server, createPairingCode(t, server, admin), "home-mac", "macos")

	heartbeat := []byte(`{"deviceID":"office-pc","name":"Office PC","platform":"windows","permission":"ready"}`)
	registered := deviceRemoteRequest(t, server, "office-pc", targetCredential, http.MethodPost, "/api/remote/hosts/heartbeat", heartbeat, "device-heartbeat-0001")
	if registered.Code != http.StatusOK {
		t.Fatalf("device heartbeat failed: %d %s", registered.Code, registered.Body.String())
	}
	impersonated := deviceRemoteRequest(t, server, "home-mac", controllerCredential, http.MethodPost, "/api/remote/hosts/heartbeat", heartbeat, "device-heartbeat-0002")
	if impersonated.Code != http.StatusForbidden {
		t.Fatalf("expected claimed identity to be rejected, got %d %s", impersonated.Code, impersonated.Body.String())
	}

	createBody := []byte(`{"targetDeviceID":"office-pc","controllerDeviceID":"home-mac","controllerSSHPublicKey":""}`)
	created := deviceRemoteRequest(t, server, "home-mac", controllerCredential, http.MethodPost, "/api/remote/sessions", createBody, "device-session-create1")
	if created.Code != http.StatusCreated {
		t.Fatalf("device session create failed: %d %s", created.Code, created.Body.String())
	}
	var session remoteSessionView
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	acceptPath := "/api/remote/sessions/" + session.ID + "/accept"
	acceptBody := []byte(`{"screenX":0,"screenY":0,"screenWidth":1920,"screenHeight":1080,"sshAuthorized":false,"error":""}`)
	wrongRole := deviceRemoteRequest(t, server, "home-mac", controllerCredential, http.MethodPost, acceptPath, acceptBody, "device-session-accept1")
	if wrongRole.Code != http.StatusForbidden {
		t.Fatalf("controller accepted its own session: %d %s", wrongRole.Code, wrongRole.Body.String())
	}
	accepted := deviceRemoteRequest(t, server, "office-pc", targetCredential, http.MethodPost, acceptPath, acceptBody, "device-session-accept2")
	if accepted.Code != http.StatusOK {
		t.Fatalf("target could not accept session: %d %s", accepted.Code, accepted.Body.String())
	}
	policy := adminRequest(t, server, admin, http.MethodPut, "/api/devices/policy", []byte(`{"allowLegacyRemoteAuth":false}`))
	if policy.Code != http.StatusOK || server.devices.legacyRemoteAuthAllowed() {
		t.Fatalf("disable legacy remote auth failed: %d %s", policy.Code, policy.Body.String())
	}
	settings, err := server.options.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	legacyBypass := legacyRemoteRequest(t, server, settings.Token, http.MethodGet, "/api/remote/devices", nil, "legacy-bypass-after-policy1")
	if legacyBypass.Code != http.StatusUnauthorized {
		t.Fatalf("legacy token bypass remained available: %d %s", legacyBypass.Code, legacyBypass.Body.String())
	}

	revoked := adminRequest(t, server, admin, http.MethodDelete, "/api/devices/home-mac", nil)
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("revoke failed: %d %s", revoked.Code, revoked.Body.String())
	}
	denied := deviceRemoteRequest(t, server, "home-mac", controllerCredential, http.MethodGet, "/api/remote/devices", nil, "device-after-revoke1")
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("revoked credential remained valid: %d %s", denied.Code, denied.Body.String())
	}
	reloaded, err := loadDeviceRegistry(server.options.DevicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.legacyRemoteAuthAllowed() {
		t.Fatal("legacy remote auth policy did not persist")
	}
}

func TestExpiredAndMalformedPairingCodesAreRejected(t *testing.T) {
	server := integrationServer(t)
	expiredCode := "AAAAA-BBBBB-CCCCC-DDDDD"
	server.enrollments[pairingCodeID(expiredCode)] = pendingEnrollment{Key: pairingCodeKey(expiredCode), ExpiresAt: time.Now().Add(-time.Second)}
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{name: "expired", body: pairingRequestBody(t, expiredCode, "office-pc", "Office", "windows", "test-expired-nonce-0001"), want: http.StatusUnauthorized},
		{name: "malformed", body: []byte(`{"codeID":"BAD","nonce":"short","proof":"bad","deviceID":"office-pc","name":"Office","platform":"windows"}`), want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/client/enroll", bytes.NewReader(test.body)))
			if response.Code != test.want {
				t.Fatalf("expected pairing rejection %d, got %d %s", test.want, response.Code, response.Body.String())
			}
		})
	}
}

func TestWrongPairingProofDoesNotConsumeCode(t *testing.T) {
	server := integrationServer(t)
	admin := loginAdmin(t, server)
	code := createPairingCode(t, server, admin)
	body := pairingRequestBody(t, code, "office-pc", "Office", "windows", "test-wrong-proof-0001")
	var request map[string]string
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	request["proof"] = strings.Repeat("00", 32)
	body, _ = json.Marshal(request)

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/client/enroll", bytes.NewReader(body)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong proof to be rejected, got %d %s", response.Code, response.Body.String())
	}
	enrollTestDevice(t, server, code, "office-pc", "windows")
}
