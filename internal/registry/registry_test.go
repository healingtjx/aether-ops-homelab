package registry

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientListImagesWithBasicAuth(t *testing.T) {
	var sawBasic bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass")) {
			sawBasic = true
		}
		switch r.URL.Path {
		case "/v2/_catalog":
			w.Write([]byte(`{"repositories":["team/app"]}`))
		case "/v2/team/app/tags/list":
			w.Write([]byte(`{"name":"team/app","tags":["v1"]}`))
		case "/v2/team/app/manifests/v1":
			if !strings.Contains(r.Header.Get("Accept"), "application/vnd.docker.distribution.manifest.v2+json") {
				t.Fatalf("missing manifest accept header: %s", r.Header.Get("Accept"))
			}
			w.Header().Set("Docker-Content-Digest", "sha256:abc")
			w.Write([]byte(`{"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"size":10},"layers":[{"size":20},{"size":30}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{RegistryURL: server.URL, Namespace: "team", AuthType: AuthBasic, Username: "user", Credential: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	images, err := client.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sawBasic {
		t.Fatal("basic auth header was not sent")
	}
	if len(images) != 1 {
		t.Fatalf("images len = %d, want 1", len(images))
	}
	if images[0].Repository != "team/app" || images[0].Tag != "v1" || images[0].Digest != "sha256:abc" || images[0].SizeBytes != 60 {
		t.Fatalf("unexpected image: %+v", images[0])
	}
}

func TestClientListImagesResolvesOCIIndexSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/_catalog":
			w.Write([]byte(`{"repositories":["team/app"]}`))
		case "/v2/team/app/tags/list":
			w.Write([]byte(`{"name":"team/app","tags":["v1"]}`))
		case "/v2/team/app/manifests/v1":
			w.Header().Set("Docker-Content-Digest", "sha256:index")
			w.Write([]byte(`{
				"mediaType":"application/vnd.oci.image.index.v1+json",
				"manifests":[
					{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:image","size":856,"platform":{"architecture":"amd64","os":"linux"}},
					{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:attestation","size":566,"platform":{"architecture":"unknown","os":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest"}}
				]
			}`))
		case "/v2/team/app/manifests/sha256:image":
			w.Write([]byte(`{"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"size":100},"layers":[{"size":200},{"size":300}]}`))
		case "/v2/team/app/manifests/sha256:attestation":
			t.Fatal("attestation manifest should not be fetched for size calculation")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{RegistryURL: server.URL, Namespace: "team"})
	if err != nil {
		t.Fatal(err)
	}
	images, err := client.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("images len = %d, want 1", len(images))
	}
	if images[0].Digest != "sha256:index" {
		t.Fatalf("digest = %s", images[0].Digest)
	}
	if images[0].SizeBytes != 600 {
		t.Fatalf("size = %d, want 600", images[0].SizeBytes)
	}
}

func TestClientGetImageFetchesOnlyRequestedManifest(t *testing.T) {
	var manifestRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/team/app/manifests/v2":
			manifestRequests++
			w.Header().Set("Docker-Content-Digest", "sha256:def")
			w.Write([]byte(`{"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"size":7},"layers":[{"size":11}]}`))
		case "/v2/_catalog", "/v2/team/app/tags/list":
			t.Fatalf("GetImage should not request %s", r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{RegistryURL: server.URL, Namespace: "team"})
	if err != nil {
		t.Fatal(err)
	}
	image, err := client.GetImage(context.Background(), "app", "v2")
	if err != nil {
		t.Fatal(err)
	}
	if manifestRequests != 1 {
		t.Fatalf("manifest requests = %d, want 1", manifestRequests)
	}
	if image.Repository != "team/app" || image.Tag != "v2" || image.Digest != "sha256:def" || image.SizeBytes != 18 {
		t.Fatalf("unexpected image: %+v", image)
	}
}

func TestClientDeleteManifestUsesDigestWithBearerAuth(t *testing.T) {
	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client, err := NewClient(Config{RegistryURL: server.URL, AuthType: AuthBearer, Credential: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteManifest(context.Background(), "team/app", "sha256:abc", "v1"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v2/team/app/manifests/sha256:abc" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("auth = %s", gotAuth)
	}
}

func TestClientFollowsBearerChallenge(t *testing.T) {
	var serverURL string
	var sawTokenBasic bool
	var retriedWithBearer bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Header.Get("Authorization") == "Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass")) {
				sawTokenBasic = true
			}
			if r.URL.Query().Get("service") != "registry.test" || r.URL.Query().Get("scope") != "registry:catalog:*" {
				t.Fatalf("unexpected token query: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"token":"challenge-token"}`))
		case "/v2/_catalog":
			if r.Header.Get("Authorization") == "Bearer challenge-token" {
				retriedWithBearer = true
				w.Write([]byte(`{"repositories":[]}`))
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+serverURL+`/token",service="registry.test",scope="registry:catalog:*"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	defer server.Close()

	client, err := NewClient(Config{RegistryURL: server.URL, AuthType: AuthBasic, Username: "user", Credential: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListImages(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sawTokenBasic {
		t.Fatal("token request did not use basic auth")
	}
	if !retriedWithBearer {
		t.Fatal("catalog request was not retried with bearer token")
	}
}
