package testrun

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Env é o contexto de resolução das expressões {{...}}. O "agora" entra por
// injeção: os testes unitários fixam a data, e a execução real usa a data do
// servidor Fluig.
type Env struct {
	Today     time.Time         // data local do servidor (só a data importa)
	Vars      map[string]string // vars do arquivo + --var
	SelfLogin string            // ator "eu"
	SelfCode  string            // userCode do ator "eu"
	RequestID int               // 0 antes do start
	Steps     map[string]int    // id do passo → nextState
	Unique    string            // valor do {{unique}} do cenário
}

const dateLayout = "02/01/2006" // dd/MM/yyyy, padrão dos cards Fluig

var reToday = regexp.MustCompile(`^today(?:\(\s*([+-]\d+)\s*\))?$`)
var reStepRef = regexp.MustCompile(`^steps\.([A-Za-z0-9_-]+)\.nextState$`)

// exprKind classifica uma expressão sem resolvê-la (validação de carga).
type exprRef struct {
	kind string // today | unique | var | actor | requestId | step
	arg  string // nome da var, do passo, ou atributo do ator
	days int
}

func parseExpr(raw string) (exprRef, error) {
	e := strings.TrimSpace(raw)
	if m := reToday.FindStringSubmatch(e); m != nil {
		r := exprRef{kind: "today"}
		if m[1] != "" {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return r, fmt.Errorf("expressão {{%s}} inválida", raw)
			}
			r.days = n
		}
		return r, nil
	}
	switch {
	case e == "unique":
		return exprRef{kind: "unique"}, nil
	case e == "requestId":
		return exprRef{kind: "requestId"}, nil
	case strings.HasPrefix(e, "var."):
		nome := strings.TrimPrefix(e, "var.")
		if nome == "" {
			return exprRef{}, fmt.Errorf("expressão {{%s}} sem nome de variável", raw)
		}
		return exprRef{kind: "var", arg: nome}, nil
	case strings.HasPrefix(e, "actors."):
		partes := strings.Split(strings.TrimPrefix(e, "actors."), ".")
		if len(partes) != 2 || (partes[1] != "login" && partes[1] != "code") {
			return exprRef{}, fmt.Errorf("expressão {{%s}} inválida; use {{actors.<apelido>.login}} ou {{actors.<apelido>.code}}", raw)
		}
		if partes[0] != ActorSelf {
			return exprRef{}, fmt.Errorf("expressão {{%s}}: ator %q desconhecido (nesta versão só existe %q)", raw, partes[0], ActorSelf)
		}
		return exprRef{kind: "actor", arg: partes[1]}, nil
	case strings.HasPrefix(e, "workday"):
		return exprRef{}, fmt.Errorf("expressão {{%s}} ainda não é suportada (fase B); use {{today(+N)}} ou {{var.<nome>}}", raw)
	}
	if m := reStepRef.FindStringSubmatch(e); m != nil {
		return exprRef{kind: "step", arg: m[1]}, nil
	}
	return exprRef{}, fmt.Errorf("expressão {{%s}} desconhecida", raw)
}

// scanExprs percorre as expressões de um texto e chama fn para cada uma.
// "{{{{" é o escape de "{{" literal.
func scanExprs(s string, fn func(raw string) (string, error)) (string, error) {
	var b strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			b.WriteString(s)
			return b.String(), nil
		}
		b.WriteString(s[:i])
		s = s[i:]
		if strings.HasPrefix(s, "{{{{") {
			b.WriteString("{{")
			s = s[4:]
			continue
		}
		j := strings.Index(s, "}}")
		if j < 0 {
			return "", fmt.Errorf("expressão sem fechamento \"}}\" em %q", s)
		}
		out, err := fn(s[2:j])
		if err != nil {
			return "", err
		}
		b.WriteString(out)
		s = s[j+2:]
	}
}

// Resolve troca as expressões do texto pelo valor no contexto.
func Resolve(s string, env *Env) (string, error) {
	return scanExprs(s, func(raw string) (string, error) {
		r, err := parseExpr(raw)
		if err != nil {
			return "", err
		}
		switch r.kind {
		case "today":
			return env.Today.AddDate(0, 0, r.days).Format(dateLayout), nil
		case "unique":
			return env.Unique, nil
		case "var":
			v, ok := env.Vars[r.arg]
			if !ok {
				return "", fmt.Errorf("variável %q não definida (use \"vars\" na suíte ou --var %s=valor)", r.arg, r.arg)
			}
			return v, nil
		case "actor":
			if r.arg == "login" {
				return env.SelfLogin, nil
			}
			return env.SelfCode, nil
		case "requestId":
			if env.RequestID == 0 {
				return "", fmt.Errorf("{{requestId}} ainda não existe: a solicitação nasce no start")
			}
			return strconv.Itoa(env.RequestID), nil
		case "step":
			v, ok := env.Steps[r.arg]
			if !ok {
				return "", fmt.Errorf("{{steps.%s.nextState}}: o passo %q ainda não rodou", r.arg, r.arg)
			}
			return strconv.Itoa(v), nil
		}
		return "", fmt.Errorf("expressão {{%s}} desconhecida", raw)
	})
}

// ResolveMap resolve todos os valores de um mapa (cópia nova).
func ResolveMap(m map[string]string, env *Env) (map[string]string, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		r, err := Resolve(v, env)
		if err != nil {
			return nil, fmt.Errorf("campo %q: %w", k, err)
		}
		out[k] = r
	}
	return out, nil
}

// UniqueValue monta o {{unique}}: "zzt" + 8 dígitos base36 do horário + o
// índice do cenário. Curto o bastante para campos de número de documento.
func UniqueValue(now time.Time, scenarioIndex int) string {
	b36 := strconv.FormatInt(now.Unix(), 36)
	if len(b36) > 8 {
		b36 = b36[len(b36)-8:]
	}
	return fmt.Sprintf("zzt%08s%d", b36, scenarioIndex)
}

// validateExpressions confere, na carga, todas as expressões da suíte:
// sintaxe, variáveis definidas e referências a passos ANTERIORES do cenário.
func validateExpressions(s *Suite) error {
	for _, sc := range s.Scenarios {
		vistos := map[string]bool{}
		check := func(where, text string, ateAqui map[string]bool) error {
			_, err := scanExprs(text, func(raw string) (string, error) {
				r, err := parseExpr(raw)
				if err != nil {
					return "", err
				}
				switch r.kind {
				case "var":
					if _, ok := s.Vars[r.arg]; !ok {
						return "", fmt.Errorf("variável %q não definida (use \"vars\" na suíte ou --var %s=valor)", r.arg, r.arg)
					}
				case "step":
					if !ateAqui[r.arg] {
						return "", fmt.Errorf("{{steps.%s.nextState}} aponta um passo que não roda antes deste", r.arg)
					}
				}
				return "", nil
			})
			if err != nil {
				return usageErr("%s: cenário %q, %s: %v", s.File, sc.Name, where, err)
			}
			return nil
		}
		for k, v := range sc.CardValues {
			if err := check("fixture "+sc.Card+" campo "+k, v, vistos); err != nil {
				return err
			}
		}
		for k, v := range sc.Fields {
			if err := check("fields."+k, v, vistos); err != nil {
				return err
			}
		}
		for _, st := range sc.Steps {
			onde := fmt.Sprintf("passo %d (%s)", st.Index, st.Name)
			for k, v := range st.Fields {
				if err := check(onde+" campo "+k, v, vistos); err != nil {
					return err
				}
			}
			for _, t := range []string{st.Comment, st.Assignee} {
				if err := check(onde, t, vistos); err != nil {
					return err
				}
			}
			// O expect avalia DEPOIS do passo: o próprio id já vale.
			depois := vistos
			if st.ID != "" {
				depois = map[string]bool{st.ID: true}
				for k := range vistos {
					depois[k] = true
				}
			}
			if e := st.Expect; e != nil {
				textos := []string{}
				if e.StateName != nil {
					textos = append(textos, *e.StateName)
				}
				if e.Assignee != nil {
					textos = append(textos, *e.Assignee)
				}
				if e.Error != nil {
					textos = append(textos, e.Error.Contains)
				}
				for _, c := range e.Card {
					textos = append(textos, c.Value)
					textos = append(textos, c.List...)
				}
				for _, t := range textos {
					if err := check(onde+" expect", t, depois); err != nil {
						return err
					}
				}
			}
			vistos = depois
		}
	}
	return nil
}

// parseDuration aceita a sintaxe do Go ("3m", "5s", "1m30s").
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("duração inválida %q", s)
	}
	return d, nil
}

func compileRegex(s string) (*regexp.Regexp, error) { return regexp.Compile(s) }
