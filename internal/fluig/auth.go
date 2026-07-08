package fluig

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Endpoints de autenticação e sessão.
const (
	pathLogin       = "/portal/api/servlet/login.do"
	pathPing        = "/portal/p/api/servlet/ping"
	pathDemoLicense = "/portal/api/servlet/license.do?demo=true"
	pathFindUser    = "/portal/api/rest/wcmservice/rest/user/findUserByLogin"
)

// Login autentica via login.do (form-urlencoded). A sessão fica nos cookies do
// jar; é considerada válida se contiver JSESSIONIDSSO ou jwt.token. Cookies de
// sessão anteriores são expirados antes, para que o resultado reflita ESTA
// tentativa (o jar é compartilhado por host+usuário no processo).
func (c *Client) Login(ctx context.Context) error {
	c.clearSessionCookies()
	form := url.Values{
		"j_username": {c.opts.Username},
		"j_password": {c.opts.Password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(pathLogin),
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao conectar em %s: %w", c.base.Host, err)
	}
	if _, err := readBody(resp, maxBodyLog); err != nil {
		return err
	}
	if resp.StatusCode >= 500 {
		return &HTTPError{StatusCode: resp.StatusCode, URL: pathLogin}
	}
	if !c.hasSessionCookies() {
		return fmt.Errorf("%w: usuário ou senha inválidos para %q em %s",
			ErrAuthFailed, c.opts.Username, c.base.Host)
	}
	return nil
}

// clearSessionCookies expira os cookies de sessão no jar.
func (c *Client) clearSessionCookies() {
	var expired []*http.Cookie
	for _, ck := range c.cookies() {
		if ck.Name == "JSESSIONIDSSO" || ck.Name == "jwt.token" {
			expired = append(expired, &http.Cookie{Name: ck.Name, Value: "", Path: "/", MaxAge: -1})
		}
	}
	if len(expired) > 0 {
		c.http.Jar.SetCookies(c.base, expired)
	}
}

// hasSessionCookies verifica se o jar contém uma sessão Fluig válida.
func (c *Client) hasSessionCookies() bool {
	for _, ck := range c.cookies() {
		if ck.Name == "JSESSIONIDSSO" || ck.Name == "jwt.token" {
			return true
		}
	}
	return false
}

// Ping valida a sessão atual; sucesso = corpo contendo "pong".
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(pathPing), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao validar a sessão em %s: %w", c.base.Host, err)
	}
	body, err := readBody(resp, maxBodyLog)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "pong") {
		return fmt.Errorf("%w: sessão inválida (ping sem pong)", ErrAuthFailed)
	}
	return nil
}

// EnsureSession garante uma sessão válida: reutiliza a sessão em cache quando o
// ping confirma; caso contrário refaz o login, aplicando o quirk de servidores
// demo (license.do?demo=true + novo login).
func (c *Client) EnsureSession(ctx context.Context) error {
	// 1. Sessão já ativa neste processo (cache em memória).
	if c.hasSessionCookies() && c.Ping(ctx) == nil {
		return nil
	}
	// 2. Sessão persistida em disco (reaproveitada entre execuções).
	if c.cache != nil && !c.hasSessionCookies() {
		if cookies := c.cache.Load(c.sessionKey); len(cookies) > 0 {
			c.http.Jar.SetCookies(c.base, cookies)
			if c.hasSessionCookies() && c.Ping(ctx) == nil {
				return nil
			}
		}
	}
	// 3. Login (com o quirk de servidores demo).
	if err := c.loginAndValidate(ctx); err != nil {
		return err
	}
	c.saveSession()
	return nil
}

// loginAndValidate faz login e valida por ping, aplicando o quirk de demo.
func (c *Client) loginAndValidate(ctx context.Context) error {
	if err := c.Login(ctx); err != nil {
		return err
	}
	if c.Ping(ctx) == nil {
		return nil
	}
	// Quirk "modo demo": login com cookies válidos mas ping falhando.
	c.requestDemoLicense(ctx)
	if err := c.Login(ctx); err != nil {
		return err
	}
	if err := c.Ping(ctx); err != nil {
		return fmt.Errorf("%w: sessão não validada mesmo após novo login", ErrAuthFailed)
	}
	return nil
}

// saveSession persiste os cookies de sessão no cache em disco, se houver.
func (c *Client) saveSession() {
	if c.cache == nil {
		return
	}
	_ = c.cache.Save(c.sessionKey, c.cookies())
}

// requestDemoLicense dispara o license.do de servidores de demonstração;
// erros são ignorados de propósito (é apenas uma tentativa de destravar o login).
func (c *Client) requestDemoLicense(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(pathDemoLicense), nil)
	if err != nil {
		return
	}
	if resp, err := c.http.Do(req); err == nil {
		_, _ = readBody(resp, maxBodyLog)
	}
}

// JWTClaims são os dados extraídos do cookie jwt.token.
type JWTClaims struct {
	Tenant int    // companyId
	Sub    string // username
}

// SessionClaims decodifica o payload do cookie jwt.token, se presente.
func (c *Client) SessionClaims() (*JWTClaims, bool) {
	for _, ck := range c.cookies() {
		if ck.Name == "jwt.token" {
			claims, err := parseJWTClaims(ck.Value)
			if err != nil {
				return nil, false
			}
			return claims, true
		}
	}
	return nil, false
}

// parseJWTClaims extrai tenant e sub do payload (base64url) de um JWT, sem
// validar assinatura — o token veio do próprio servidor.
func parseJWTClaims(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("jwt malformado")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("payload do jwt inválido: %w", err)
	}
	var raw struct {
		Tenant any    `json:"tenant"`
		Sub    string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("payload do jwt não é JSON: %w", err)
	}
	claims := &JWTClaims{Sub: raw.Sub}
	switch t := raw.Tenant.(type) {
	case float64:
		claims.Tenant = int(t)
	case string:
		claims.Tenant, _ = strconv.Atoi(t)
	}
	return claims, nil
}

// ResolveUserCode devolve o userCode real do usuário autenticado (o
// colleagueId/publisherId usado pelo ECMCardIndexService). O login puro NÃO
// serve — o servidor devolve lista vazia (validado na homologação).
// O resultado é cacheado por Client.
func (c *Client) ResolveUserCode(ctx context.Context) (string, error) {
	if c.userCode != "" {
		return c.userCode, nil
	}
	if err := c.EnsureSession(ctx); err != nil {
		return "", err
	}
	user, err := c.FindUserByLogin(ctx, c.opts.Username)
	if err != nil {
		return "", err
	}
	code := user.Code
	if code == "" {
		code = c.opts.Username // fallback defensivo
	}
	c.userCode = code
	return code, nil
}

// User são os dados retornados por findUserByLogin. Raw preserva a resposta
// completa para a saída --json.
type User struct {
	FullName string
	Email    string
	Code     string
	Raw      map[string]any
}

// FindUserByLogin consulta os dados do usuário autenticado (usado pelo
// `server test` e para preencher o userCode da config).
func (c *Client) FindUserByLogin(ctx context.Context, login string) (*User, error) {
	endpoint := c.url(pathFindUser) + "?login=" + url.QueryEscape(login)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar o usuário em %s: %w", c.base.Host, err)
	}
	body, err := readBody(resp, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: usuário %q", ErrNotFound, login)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: pathFindUser, Body: truncate(body, 512)}
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, fmt.Errorf("resposta inesperada de findUserByLogin: %w", err)
	}
	return parseUser(raw), nil
}

// parseUser extrai os campos usuais de forma defensiva. Formato confirmado na
// homologação (Fluig 1.8.2, 2026-07-07): usuário envelopado em "content", com
// "fullName", "email" e "userCode"; mantemos sinônimos por variação de versão.
func parseUser(raw map[string]any) *User {
	obj := raw
	if content, ok := raw["content"].(map[string]any); ok {
		obj = content
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := obj[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	return &User{
		FullName: str("fullName", "colleagueName", "name"),
		Email:    str("email", "mail"),
		Code:     str("userCode", "colleagueId", "code"),
		Raw:      raw,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
