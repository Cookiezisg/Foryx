package reqctx

import (
	"context"
	"testing"
)

func TestSetGetLocale_RoundTrip(t *testing.T) {
	ctx := SetLocale(context.Background(), LocaleEn)
	if got := GetLocale(ctx); got != LocaleEn {
		t.Errorf("got %q, want %q", got, LocaleEn)
	}
}

func TestGetLocale_MissingReturnsDefault(t *testing.T) {
	if got := GetLocale(context.Background()); got != DefaultLocale {
		t.Errorf("got %q, want default %q", got, DefaultLocale)
	}
}

func TestGetLocale_UnsupportedFallsBackToDefault(t *testing.T) {
	ctx := SetLocale(context.Background(), Locale("fr-FR"))
	if got := GetLocale(ctx); got != DefaultLocale {
		t.Errorf("got %q, want default %q", got, DefaultLocale)
	}
}

func TestGetLocale_PrivateKeyIsolation(t *testing.T) {
	//lint:ignore SA1029 intentional: simulating external code that uses a raw string key
	ctx := context.WithValue(context.Background(), "locale", "en")
	if got := GetLocale(ctx); got != DefaultLocale {
		t.Errorf("string-keyed value leaked: got %q, want default %q", got, DefaultLocale)
	}
}

func TestLocale_IsSupported(t *testing.T) {
	cases := []struct {
		in   Locale
		want bool
	}{
		{LocaleZhCN, true},
		{LocaleEn, true},
		{Locale(""), false},
		{Locale("fr-FR"), false},
		{Locale("zh"), false},
	}
	for _, c := range cases {
		if got := c.in.IsSupported(); got != c.want {
			t.Errorf("%q.IsSupported(): got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDefaultLocale_IsSupported(t *testing.T) {
	if !DefaultLocale.IsSupported() {
		t.Errorf("DefaultLocale %q is not in IsSupported() list", DefaultLocale)
	}
}
