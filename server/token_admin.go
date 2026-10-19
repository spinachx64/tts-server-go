package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jing332/tts-server-go/internal/tokenstore"
	log "github.com/sirupsen/logrus"
)

func (s *GracefulServer) validAdmin(r *http.Request) bool {
	authorization := r.Header.Get("Authorization")
	scheme, credential, ok := strings.Cut(authorization, " ")
	return s.AdminToken != "" && ok && strings.EqualFold(scheme, "Bearer") && subtle.ConstantTimeCompare([]byte(credential), []byte(s.AdminToken)) == 1
}
func (s *GracefulServer) tokenAdminHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.AdminToken == "" || s.TokenManager == nil {
		jsonError(w, 404, "未启用Token管理接口")
		return
	}
	if !s.validAdmin(r) {
		jsonError(w, 401, "无效的管理Token")
		return
	}
	switch r.Method {
	case http.MethodPost:
		contentType, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr != nil || contentType != "application/json" {
			jsonError(w, 415, "Content-Type 必须是 application/json")
			return
		}
		var input struct {
			Username  string    `json:"username"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			jsonError(w, 400, "请求格式错误，expires_at 使用 RFC3339 时间")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			jsonError(w, 400, "请求只能包含一个 JSON 对象")
			return
		}
		input.Username = strings.TrimSpace(input.Username)
		if input.Username == "" || len(input.Username) > 128 || strings.ContainsFunc(input.Username, unicode.IsControl) {
			jsonError(w, 400, "username 必填，最多128字节，不能包含控制字符")
			return
		}
		if !input.ExpiresAt.After(time.Now()) {
			jsonError(w, 400, "expires_at 必须是未来时间")
			return
		}
		token, err := s.TokenManager.Create(r.Context(), input.Username, input.ExpiresAt)
		if err != nil {
			s.adminDBError(w, err)
			return
		}
		jsonResponse(w, 201, map[string]any{"token": token, "username": input.Username, "expires_at": time.Unix(input.ExpiresAt.Unix(), 0).UTC(), "enabled": true})
	case http.MethodGet, http.MethodDelete:
		token := r.Header.Get("Token")
		if token == "" {
			jsonError(w, 400, "Token 请求头需提供待查询或删除的用户Token")
			return
		}
		if r.Method == http.MethodGet {
			info, err := s.TokenManager.Info(r.Context(), token)
			if errors.Is(err, tokenstore.ErrNotFound) {
				jsonError(w, 404, "Token不存在")
				return
			}
			if err != nil {
				s.adminDBError(w, err)
				return
			}
			jsonResponse(w, 200, info)
		} else {
			err := s.TokenManager.Delete(r.Context(), token)
			if errors.Is(err, tokenstore.ErrNotFound) {
				jsonError(w, 404, "Token不存在")
				return
			}
			if err != nil {
				s.adminDBError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		w.Header().Set("Allow", "POST, GET, DELETE")
		jsonError(w, 405, "请使用 POST、GET 或 DELETE")
	}
}
func (s *GracefulServer) adminDBError(w http.ResponseWriter, err error) {
	log.WithError(err).Error("Token管理数据库操作失败")
	jsonError(w, 503, "Token管理暂不可用")
}
