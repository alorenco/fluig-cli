package sqlsplit

import "testing"

func TestSplit(t *testing.T) {
	casos := []struct {
		nome   string
		script string
		quer   []string
	}{
		{
			nome:   "vazio",
			script: "   \n\n  ",
			quer:   nil,
		},
		{
			nome:   "uma instrução sem ponto e vírgula",
			script: "select 1",
			quer:   []string{"select 1"},
		},
		{
			nome:   "duas instruções",
			script: "select 1; select 2;",
			quer:   []string{"select 1", "select 2"},
		},
		{
			nome:   "ponto e vírgula final opcional",
			script: "select 1;\nselect 2",
			quer:   []string{"select 1", "select 2"},
		},
		{
			// O caso que quebra o split ingênuo por ";".
			nome:   "ponto e vírgula dentro de literal",
			script: "select 'a;b' as x; select 2",
			quer:   []string{"select 'a;b' as x", "select 2"},
		},
		{
			nome:   "aspas simples dobradas dentro do literal",
			script: "select 'o''brien; ainda no literal' as x; select 2",
			quer:   []string{"select 'o''brien; ainda no literal' as x", "select 2"},
		},
		{
			nome:   "colchete com ponto e vírgula",
			script: "select * from [tabela;estranha]; select 2",
			quer:   []string{"select * from [tabela;estranha]", "select 2"},
		},
		{
			nome:   "colchete escapado",
			script: "select [col]]; nome] from t",
			quer:   []string{"select [col]]; nome] from t"},
		},
		{
			nome:   "identificador entre aspas duplas",
			script: `select "col;1" from t; select 2`,
			quer:   []string{`select "col;1" from t`, "select 2"},
		},
		{
			nome:   "comentário de linha com ponto e vírgula",
			script: "select 1 -- comentário; não separa\n; select 2",
			quer:   []string{"select 1 -- comentário; não separa", "select 2"},
		},
		{
			nome:   "comentário de bloco com ponto e vírgula",
			script: "select /* a; b */ 1; select 2",
			quer:   []string{"select /* a; b */ 1", "select 2"},
		},
		{
			nome:   "comentário de bloco aninhado (T-SQL)",
			script: "select /* a /* b; */ c */ 1; select 2",
			quer:   []string{"select /* a /* b; */ c */ 1", "select 2"},
		},
		{
			nome:   "instrução só com comentário é descartada",
			script: "-- cabeçalho do script\n/* nada aqui */\n; select 1",
			quer:   []string{"select 1"},
		},
		{
			nome:   "GO sozinho separa lote",
			script: "select 1\nGO\nselect 2\ngo\n",
			quer:   []string{"select 1", "select 2"},
		},
		{
			nome:   "GO com comentário na mesma linha",
			script: "select 1\nGO -- fim do lote\nselect 2",
			quer:   []string{"select 1", "select 2"},
		},
		{
			nome:   "GOTO não é separador",
			script: "select 1 from goal; select goto_x from t",
			quer:   []string{"select 1 from goal", "select goto_x from t"},
		},
		{
			nome:   "GO no meio da linha não separa",
			script: "select 1 GO 2",
			quer:   []string{"select 1 GO 2"},
		},
		{
			nome:   "CTE em várias linhas conta como uma instrução",
			script: "with x as (\n  select 1 as n\n)\nselect n from x;\nselect 2",
			quer:   []string{"with x as (\n  select 1 as n\n)\nselect n from x", "select 2"},
		},
		{
			nome:   "múltiplos separadores seguidos",
			script: "select 1;;\n;select 2;",
			quer:   []string{"select 1", "select 2"},
		},
		{
			// O helper decide pela PRIMEIRA palavra do texto recebido, sem pular
			// comentário: um `-- nota` acima do select fazia o servidor recusar a
			// consulta como se fosse escrita (medido na homologação em 2026-07-29).
			nome:   "comentário de linha ANTES do SQL é removido",
			script: "-- nota do autor\nselect 1;\n-- outra nota\nselect 2",
			quer:   []string{"select 1", "select 2"},
		},
		{
			nome:   "comentário de bloco ANTES do SQL é removido",
			script: "/* bloco\n   multilinha */\nselect count(*) as n from t",
			quer:   []string{"select count(*) as n from t"},
		},
		{
			nome:   "comentário interno e final são preservados",
			script: "-- antes\nselect /* meio */ 1 -- depois",
			quer:   []string{"select /* meio */ 1 -- depois"},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := Split(c.script)
			if len(got) != len(c.quer) {
				t.Fatalf("%d instrução(ões), quer %d: %+v", len(got), len(c.quer), got)
			}
			for i := range got {
				if got[i].SQL != c.quer[i] {
					t.Errorf("instrução %d = %q, quer %q", i+1, got[i].SQL, c.quer[i])
				}
			}
		})
	}
}

// A linha reportada é a do primeiro trecho de SQL da instrução — não a do
// comentário que a antecede. É o que localiza o erro num script de 200 linhas.
func TestSplitLinhas(t *testing.T) {
	script := "-- cabeçalho\n\nselect 1;\n\n-- etapa 2\nselect 2;\nGO\nselect 3"
	got := Split(script)
	quer := []int{3, 6, 8}
	if len(got) != len(quer) {
		t.Fatalf("%d instruções: %+v", len(got), got)
	}
	for i := range got {
		if got[i].Line != quer[i] {
			t.Errorf("instrução %d na linha %d, quer %d (%q)", i+1, got[i].Line, quer[i], got[i].SQL)
		}
	}
}

// Uma instrução com literal multilinha não pode desalinhar o contador de linhas.
func TestSplitLinhasComLiteralMultilinha(t *testing.T) {
	script := "select '\n\n' as x;\nselect 2"
	got := Split(script)
	if len(got) != 2 {
		t.Fatalf("%d instruções: %+v", len(got), got)
	}
	if got[1].Line != 4 {
		t.Errorf("segunda instrução na linha %d, quer 4", got[1].Line)
	}
}

func TestFirstLine(t *testing.T) {
	casos := []struct {
		sql  string
		max  int
		quer string
	}{
		{sql: "select 1", quer: "select 1"},
		{sql: "-- comentário\nselect 1", quer: "select 1"},
		{sql: "/* bloco */\n\nselect 1 from t", quer: "select 1 from t"},
		{sql: "  select 1  ", quer: "select 1"},
		// Corta em runas, não em bytes: o acento não pode virar meio caractere.
		{sql: "select ação from t", max: 10, quer: "select açã…"},
		{sql: "select 1234567890 from t", max: 13, quer: "select 123456…"},
		{sql: "select 1", max: 100, quer: "select 1"},
		// Só comentário: melhor devolver o comentário do que string vazia.
		{sql: "-- só comentário", quer: "-- só comentário"},
		// Comentário de bloco MULTILINHA: a heurística de prefixo de linha pegava
		// "com ; dentro */" como se fosse a instrução (visto na homologação).
		{sql: "/* comentário de bloco\n   com ; dentro */\nselect count(*) as n from t",
			quer: "select count(*) as n from t"},
		{sql: "-- cabeçalho\n/* nota */ select 1 from t", quer: "select 1 from t"},
		// Comentário depois do SQL não muda a identificação.
		{sql: "select 1 -- nota", quer: "select 1"},
	}
	for _, c := range casos {
		if got := FirstLine(c.sql, c.max); got != c.quer {
			t.Errorf("FirstLine(%q, %d) = %q, quer %q", c.sql, c.max, got, c.quer)
		}
	}
}
