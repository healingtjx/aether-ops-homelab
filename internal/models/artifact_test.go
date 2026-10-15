package models

import "testing"

func TestArtifactImageRefParts(t *testing.T) {
	artifact := Artifact{
		RegistryURL: "https://crpi-cin2gcjtiq7btawt.ap-southeast-1.personal.cr.aliyuncs.com/",
		Repository:  "/bs_code/bs-blade-log/",
		Tag:         "26.0409.0",
	}

	if got := artifact.RegistryHost(); got != "crpi-cin2gcjtiq7btawt.ap-southeast-1.personal.cr.aliyuncs.com" {
		t.Fatalf("RegistryHost() = %q", got)
	}
	if got := artifact.RepositoryTagRef(); got != "bs_code/bs-blade-log:26.0409.0" {
		t.Fatalf("RepositoryTagRef() = %q", got)
	}
	if got := artifact.ImageRef(); got != "crpi-cin2gcjtiq7btawt.ap-southeast-1.personal.cr.aliyuncs.com/bs_code/bs-blade-log:26.0409.0" {
		t.Fatalf("ImageRef() = %q", got)
	}
}
