package soap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Resposta REAL do saveAndSendTask em modo gestor (produção, 2026-08-25).
func TestParseSaveAndSendTask(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "soap_saveAndSendTask_manager.xml"))
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := ParseSaveAndSendTask(body)
	if err != nil {
		t.Fatal(err)
	}
	if pairs["iTask"] != "17" || pairs["cDestino"] != "[Pool:Role:cabine_fiscal]" || pairs["WDNrDocto"] != "1238303" {
		t.Errorf("pares: %v", pairs)
	}
	if !strings.Contains(pairs["processLink"], "linkSequence=83") {
		t.Errorf("processLink: %q", pairs["processLink"])
	}
}

func TestParseSaveAndSendTaskFault(t *testing.T) {
	body := []byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>` +
		`<soap:Fault><faultcode>soap:Server</faultcode><faultstring>boom</faultstring></soap:Fault></soap:Body></soap:Envelope>`)
	if _, err := ParseSaveAndSendTask(body); err == nil || err.Error() != "boom" {
		t.Fatalf("quer Fault, veio %v", err)
	}
}

func TestBuildSaveAndSendTaskOrdemDasParts(t *testing.T) {
	env, err := BuildSaveAndSendTask(1, "admin", "", "uc-admin", 228691, 80, nil, "c", true, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := string(env)
	// A ordem das parts segue o WSDL (RPC/literal casa por posição+nome).
	order := []string{"<username>", "<password>", "<companyId>", "<processInstanceId>", "<choosedState>",
		"<colleagueIds>", "<comments>", "<userId>", "<completeTask>", "<attachments>", "<cardData>", "<appointment>",
		"<managerMode>", "<threadSequence>"}
	last := -1
	for _, tag := range order {
		i := strings.Index(s, tag)
		if i < 0 || i < last {
			t.Fatalf("part %s fora de ordem/ausente em:\n%s", tag, s)
		}
		last = i
	}
	if !strings.Contains(s, "<managerMode>true</managerMode>") {
		t.Errorf("managerMode: %s", s)
	}
}
