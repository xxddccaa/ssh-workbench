package bootstrap

import "testing"

func TestConfigurationValidation(test *testing.T) {
	for _, config := range []Config{
		{Listen: "0.0.0.0:8622", DataDir: "private", Remote: true},
		{Listen: "0.0.0.0:8622", DataDir: "private", Remote: true, Origin: "http://example.test", TLSCert: "cert.pem", TLSKey: "key.pem"},
		{Listen: "0.0.0.0:8622", DataDir: "private", Remote: true, Origin: "https://example.test", TLSCert: "cert.pem", TLSKey: "key.pem", DevOrigin: "http://127.0.0.1:5173"},
		{Listen: "invalid", DataDir: "private"},
		{Listen: "127.0.0.1:8622", DataDir: ""},
	} {
		if err := config.Validate(); err == nil {
			test.Fatalf("accepted invalid config: %+v", config)
		}
	}
	for _, config := range []Config{
		{Listen: "127.0.0.1:8622", DataDir: "private"},
		{Listen: "0.0.0.0:8622", DataDir: "private"},
		{Listen: "[::]:8622", DataDir: "private"},
	} {
		if err := config.Validate(); err != nil {
			test.Fatalf("rejected valid direct-open config %+v: %v", config, err)
		}
	}
}
