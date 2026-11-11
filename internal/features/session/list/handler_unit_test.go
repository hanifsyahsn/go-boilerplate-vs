package list

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestHandler_List_U(t *testing.T) {
	sid := uuid.MustParse("6a1f0c9e-3b52-4d8e-9a10-5c2e7f4b8d31")
	at := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	one := []shared.SessionResource{{ID: sid.String(), UserAgent: "Laptop", ClientIP: "10.0.0.1", Trusted: true,
		CreatedAt: at, LastUsedAt: at, Current: true}}

	testCases := []struct {
		name          string
		query         string
		stub          func(s *MockS)
		wantCode      int
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name: "the caller's sessions as a sessions collection, newest use first by default, a trusted one's expires_at null",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), int64(7), sid, "-last_used_at").Return(one, nil)
			},
			wantCode: http.StatusOK,
			wantInBody: []string{`"type":"sessions"`, `"id":"` + sid.String() + `"`, `"current":true`, `"trusted":true`,
				`"expires_at":null`, `"user_agent":"Laptop"`, `"client_ip":"10.0.0.1"`, `"links":{"self":"/api/me/sessions"}`},
		},
		{
			name:  "sorted by creation, oldest first",
			query: "?sort=created_at",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), int64(7), sid, "created_at").Return(one, nil)
			},
			wantCode: http.StatusOK,
		},
		{
			name:  "a sparse fieldset",
			query: "?fields%5Bsessions%5D=current",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), int64(7), sid, "-last_used_at").Return(one, nil)
			},
			wantCode:      http.StatusOK,
			wantInBody:    []string{`"attributes":{"current":true}`},
			wantNotInBody: []string{`"user_agent"`},
		},
		{name: "a sort it doesn't allow is a 400", query: "?sort=user_agent", stub: func(*MockS) {}, wantCode: http.StatusBadRequest},
		{name: "pagination is not supported", query: "?page%5Bsize%5D=1", stub: func(*MockS) {}, wantCode: http.StatusBadRequest},
		{name: "include is not supported", query: "?include=user", stub: func(*MockS) {}, wantCode: http.StatusBadRequest},
		{
			name: "a service error is a 500",
			stub: func(s *MockS) {
				s.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctrl := gomock.NewController(t)
			svc := NewMockS(ctrl)
			tc.stub(svc)

			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(internalsharedconstant.UserIDKey, int64(7))
				c.Set(internalsharedconstant.SessionIDKey, sid)
				c.Next()
			})
			Route(r.Group("/api"), NewHandler(svc))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/me/sessions"+tc.query, nil))

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			for _, s := range tc.wantInBody {
				require.Contains(t, w.Body.String(), s)
			}
			for _, s := range tc.wantNotInBody {
				require.NotContains(t, w.Body.String(), s)
			}
		})
	}
}
