package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseCSV(t *testing.T) {
	got, err := parseCSV("127.0.0.1:9100, 127.0.0.1:9101,127.0.0.1:9102", 3)
	if err != nil {
		t.Fatalf("parseCSV() error = %v", err)
	}

	want := []string{
		"127.0.0.1:9100",
		"127.0.0.1:9101",
		"127.0.0.1:9102",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCSV() = %#v, want %#v", got, want)
	}
}

func TestParseCSVRejectsWrongCount(t *testing.T) {
	_, err := parseCSV("127.0.0.1:9100,127.0.0.1:9101", 3)
	if err == nil {
		t.Fatal("parseCSV() unexpectedly accepted the wrong number of addresses")
	}
}

func TestParseCSVRejectsInvalidAddress(t *testing.T) {
	_, err := parseCSV("127.0.0.1:9100,bad-address,127.0.0.1:9102", 3)
	if err == nil {
		t.Fatal("parseCSV() unexpectedly accepted an invalid address")
	}
}

func TestPeerAddressesDefault(t *testing.T) {
	got, err := peerAddresses("", 3)
	if err != nil {
		t.Fatalf("peerAddresses() error = %v", err)
	}

	want := []string{
		"127.0.0.1:9201",
		"127.0.0.1:9202",
		"127.0.0.1:9203",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("peerAddresses() = %#v, want %#v", got, want)
	}
}

func TestPeerAddressesCustom(t *testing.T) {
	got, err := peerAddresses("127.0.0.1:9301,127.0.0.1:9302", 2)
	if err != nil {
		t.Fatalf("peerAddresses() error = %v", err)
	}

	want := []string{
		"127.0.0.1:9301",
		"127.0.0.1:9302",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("peerAddresses() = %#v, want %#v", got, want)
	}
}

func TestParseGatewayKVPath(t *testing.T) {
	id, key, err := parseGatewayKVPath("/gateways/2/kv/demo")
	if err != nil {
		t.Fatalf("parseGatewayKVPath() error = %v", err)
	}

	if id != 2 {
		t.Fatalf("gateway id = %d, want 2", id)
	}
	if key != "demo" {
		t.Fatalf("key = %q, want %q", key, "demo")
	}
}

func TestParseGatewayKVPathDecodesEscapedKey(t *testing.T) {
	id, key, err := parseGatewayKVPath("/gateways/3/kv/user%3A42")
	if err != nil {
		t.Fatalf("parseGatewayKVPath() error = %v", err)
	}

	if id != 3 {
		t.Fatalf("gateway id = %d, want 3", id)
	}
	if key != "user:42" {
		t.Fatalf("key = %q, want %q", key, "user:42")
	}
}

func TestParseGatewayKVPathRejectsInvalidPath(t *testing.T) {
	_, _, err := parseGatewayKVPath("/gateways/2/not-kv/demo")
	if err == nil {
		t.Fatal("parseGatewayKVPath() unexpectedly accepted an invalid path")
	}
}

func TestParseGatewayKVPathRejectsInvalidGatewayID(t *testing.T) {
	_, _, err := parseGatewayKVPath("/gateways/not-a-number/kv/demo")
	if err == nil {
		t.Fatal("parseGatewayKVPath() unexpectedly accepted an invalid gateway id")
	}
}

func TestDecodeKeyRejectsEmptyKey(t *testing.T) {
	_, err := decodeKey("")
	if err == nil {
		t.Fatal("decodeKey() unexpectedly accepted an empty key")
	}
}

func TestClusterInfoFromConfig(t *testing.T) {
	ssds := []string{
		"127.0.0.1:9100",
		"127.0.0.1:9101",
		"127.0.0.1:9102",
	}
	peers := []string{
		"127.0.0.1:9201",
		"127.0.0.1:9202",
	}

	got := clusterInfoFromConfig("127.0.0.1:18080", ssds, peers)

	if got.HTTPAddr != "127.0.0.1:18080" {
		t.Fatalf("HTTPAddr = %q, want %q", got.HTTPAddr, "127.0.0.1:18080")
	}
	if !reflect.DeepEqual(got.SSDs, ssds) {
		t.Fatalf("SSDs = %#v, want %#v", got.SSDs, ssds)
	}
	if len(got.Gateways) != 2 {
		t.Fatalf("gateways length = %d, want 2", len(got.Gateways))
	}
	if got.Gateways[0].ID != 1 || got.Gateways[0].PeerAddr != "127.0.0.1:9201" {
		t.Fatalf("gateway 1 info = %#v", got.Gateways[0])
	}
	if got.Gateways[1].ID != 2 || got.Gateways[1].PeerAddr != "127.0.0.1:9202" {
		t.Fatalf("gateway 2 info = %#v", got.Gateways[1])
	}
}

func TestHealthzRoute(t *testing.T) {
	api := &app{}
	handler := api.routes(clusterInfo{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("status body = %#v, want status=ok", body)
	}
}

func TestGatewaysRoute(t *testing.T) {
	info := clusterInfo{
		HTTPAddr: "127.0.0.1:18080",
		SSDs: []string{
			"127.0.0.1:9100",
			"127.0.0.1:9101",
			"127.0.0.1:9102",
		},
		Gateways: []gatewayInfo{
			{ID: 1, PeerAddr: "127.0.0.1:9201"},
			{ID: 2, PeerAddr: "127.0.0.1:9202"},
		},
	}

	api := &app{}
	handler := api.routes(info)

	req := httptest.NewRequest(http.MethodGet, "/gateways", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got clusterInfo
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if !reflect.DeepEqual(got, info) {
		t.Fatalf("cluster info = %#v, want %#v", got, info)
	}
}

func TestMissingGatewayRouteReturns404(t *testing.T) {
	api := &app{
		gateways: nil,
		cfg:      config{},
	}
	handler := api.routes(clusterInfo{})

	req := httptest.NewRequest(http.MethodGet, "/gateways/9/kv/demo", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
