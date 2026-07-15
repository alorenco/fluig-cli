package soap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseReplacementList(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "soap_getReplacementsOfUser.xml"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	list, err := ParseReplacementList(body)
	if err != nil {
		t.Fatalf("ParseReplacementList: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2 substituições, veio %d", len(list))
	}
	if list[0].ColleagueID != "zz_fluigcli_test_titular" || list[0].ReplacementID != "zz_fluigcli_test_subst" {
		t.Errorf("item[0] inesperado: %+v", list[0])
	}
	if !list[0].ViewWorkflowTasks || list[0].ViewGEDTasks {
		t.Errorf("flags do item[0] inesperadas: %+v", list[0])
	}
	if list[1].StartDate != "2025-01-15T00:00:00-04:00" || !list[1].ViewGEDTasks || list[1].ViewWorkflowTasks {
		t.Errorf("item[1] inesperado: %+v", list[1])
	}
}

func TestParseReplacementListEmptyFault(t *testing.T) {
	// Lista vazia vem como fault WS-I BP R2211 (result null) — não é erro.
	const nullFault = `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Server</faultcode><faultstring>Cannot write part result. RPC/Literal parts cannot be null. (WS-I BP R2211)</faultstring></soap:Fault></soap:Body></soap:Envelope>`
	list, err := ParseReplacementList([]byte(nullFault))
	if err != nil {
		t.Fatalf("fault de result null deveria virar lista vazia, veio erro: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("esperava lista vazia, veio %d", len(list))
	}
}

func TestParseReplacementStatus(t *testing.T) {
	ok := `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns1:createColleagueReplacementResponse xmlns:ns1="http://ws.foundation.ecm.technology.totvs.com/"><result>OK</result></ns1:createColleagueReplacementResponse></soap:Body></soap:Envelope>`
	res, err := ParseReplacementStatus([]byte(ok))
	if err != nil || res != "OK" {
		t.Fatalf("status OK: res=%q err=%v", res, err)
	}

	nok := `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns1:createColleagueReplacementResponse xmlns:ns1="http://ws.foundation.ecm.technology.totvs.com/"><result>NOK Já existe uma Substituição para estes usuários e para este mesmo Período</result></ns1:createColleagueReplacementResponse></soap:Body></soap:Envelope>`
	res, err = ParseReplacementStatus([]byte(nok))
	if err != nil {
		t.Fatalf("NOK não deveria ser fault: %v", err)
	}
	if !strings.HasPrefix(res, "NOK") {
		t.Fatalf("esperava NOK, veio %q", res)
	}
}

func TestBuildReplacementEnvelopeOrder(t *testing.T) {
	// A ordem dos campos do DTO precisa seguir o sequence do WSDL.
	env, err := BuildCreateReplacement("", "", ColleagueReplacement{
		ColleagueID: "tit", ReplacementID: "sub", CompanyID: 1,
		StartDate: "2026-07-16T00:00:00", FinalDate: "2026-07-20T00:00:00",
		ViewWorkflowTasks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(env)
	if !strings.Contains(s, "<ws:createColleagueReplacement>") {
		t.Errorf("faltou o elemento raiz da operação:\n%s", s)
	}
	if !strings.Contains(s, "<colleagueReplacement>") || !strings.Contains(s, "<colleagueId>tit</colleagueId>") {
		t.Errorf("DTO malformado:\n%s", s)
	}
	// colleagueId deve vir antes de replacementId (ordem do sequence).
	if strings.Index(s, "<colleagueId>") > strings.Index(s, "<replacementId>") {
		t.Errorf("ordem dos campos do DTO fora do sequence do WSDL:\n%s", s)
	}
}
