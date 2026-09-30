package config

import "testing"

func TestRedactDSN(t *testing.T) {
	cases := map[string]string{
		"":                                  "",
		"/var/lib/iot-platform/platform.db": "/var/lib/iot-platform/platform.db",
		"postgres://iot:s3cret@db:5432/iot?sslmode=disable": "postgres://iot:xxxxx@db:5432/iot?sslmode=disable",
		"postgres://iot@db/iot":                             "postgres://iot@db/iot",
		"host=db user=iot password=s3cret dbname=iot":       "host=db user=iot password=xxxxx dbname=iot",
		"host=db password='a b' dbname=iot":                 "host=db password=xxxxx dbname=iot",
	}
	for in, want := range cases {
		if got := RedactDSN(in); got != want {
			t.Errorf("RedactDSN(%q)=%q want %q", in, got, want)
		}
	}
}
