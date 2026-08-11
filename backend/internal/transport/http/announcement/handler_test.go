package announcement

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseAnnouncementIDUsesPositiveInt64Range(t *testing.T) {
	if id, ok := parseAnnouncementID(strconv.FormatInt(math.MaxInt64, 10)); !ok || uint64(id) != math.MaxInt64 {
		t.Fatalf("max int64 rejected: id=%d ok=%v", id, ok)
	}
	for _, value := range []string{"0", "-1", "9223372036854775808"} {
		if _, ok := parseAnnouncementID(value); ok {
			t.Fatalf("invalid announcement ID accepted: %q", value)
		}
	}
}

func TestCloseAnnouncementRejectsInt64Overflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	context.Request = httptest.NewRequest(http.MethodPost, "/announcements/9223372036854775808/close", nil)
	context.Params = gin.Params{{Key: "id", Value: "9223372036854775808"}}

	(&Handler{}).CloseAnnouncement(context)
	if writer.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", writer.Code, http.StatusBadRequest)
	}
}
