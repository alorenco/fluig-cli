package fluig

import "testing"

func TestParseServerVersion(t *testing.T) {
	cases := []struct {
		name              string
		body              string
		wantRaw           string
		wantMajor, wantMi int
	}{
		{
			name:      "voyager 2.0 (homologação)",
			body:      `{"value":"TOTVS Fluig Plataforma - Voyager 2.0.0-260707"}`,
			wantRaw:   "TOTVS Fluig Plataforma - Voyager 2.0.0-260707",
			wantMajor: 2, wantMi: 0,
		},
		{
			name:      "crystal mist 1.8 (produção)",
			body:      `{"value":"TOTVS Fluig Plataforma - Crystal Mist 1.8.2-260707"}`,
			wantRaw:   "TOTVS Fluig Plataforma - Crystal Mist 1.8.2-260707",
			wantMajor: 1, wantMi: 8,
		},
		{
			name:      "campo version alternativo",
			body:      `{"version":"1.7.0"}`,
			wantRaw:   "1.7.0",
			wantMajor: 1, wantMi: 7,
		},
		{
			name:      "string JSON solta",
			body:      `"2.1.3"`,
			wantRaw:   "2.1.3",
			wantMajor: 2, wantMi: 1,
		},
		{
			name:    "corpo sem número → desconhecida",
			body:    `{"value":"sem versão aqui"}`,
			wantRaw: "sem versão aqui",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := parseServerVersion([]byte(tc.body))
			if v.Raw != tc.wantRaw {
				t.Errorf("Raw = %q, quer %q", v.Raw, tc.wantRaw)
			}
			if v.Major != tc.wantMajor || v.Minor != tc.wantMi {
				t.Errorf("versão = %d.%d, quer %d.%d", v.Major, v.Minor, tc.wantMajor, tc.wantMi)
			}
		})
	}
}

func TestServerVersionAtLeast(t *testing.T) {
	v20 := ServerVersion{Raw: "Voyager 2.0.0", Major: 2, Minor: 0}
	v18 := ServerVersion{Raw: "Crystal Mist 1.8.2", Major: 1, Minor: 8}
	unknown := ServerVersion{}

	if !v20.AtLeast(2, 0) {
		t.Error("2.0 deveria ser >= 2.0")
	}
	if v18.AtLeast(2, 0) {
		t.Error("1.8 NÃO deveria ser >= 2.0")
	}
	if !v18.AtLeast(1, 8) || v18.AtLeast(1, 9) {
		t.Error("comparação de minor errada em 1.8")
	}
	// Versão desconhecida é tratada como antiga (nunca satisfaz >= 1.0+).
	if unknown.AtLeast(2, 0) || unknown.AtLeast(1, 0) {
		t.Error("versão desconhecida não deveria satisfazer >= 1.0")
	}
	if unknown.String() != "desconhecida" {
		t.Errorf("String() de versão vazia = %q", unknown.String())
	}
}
