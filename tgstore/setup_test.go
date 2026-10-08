package tgstore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMeAndFindChat(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/botTOKEN/getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"username":"my_bot"}}`)
		case "/botTOKEN/getUpdates":
			asked++
			if asked == 1 {
				// Nothing for a channel yet, only a person saying hello.
				fmt.Fprint(w, `{"ok":true,"result":[{"update_id":7,"message":{"chat":{"id":5,"type":"private","first_name":"Ada"}}}]}`)
				return
			}
			if r.FormValue("offset") != "8" {
				t.Errorf("asked again from offset %s, want 8", r.FormValue("offset"))
			}
			fmt.Fprint(w, `{"ok":true,"result":[
				{"update_id":8,"channel_post":{"chat":{"id":-100,"type":"channel","title":"old"}}},
				{"update_id":9,"my_chat_member":{"chat":{"id":-200,"type":"channel","title":"files"}}}]}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"ok":false,"description":"Unauthorized"}`)
		}
	}))
	defer server.Close()
	store := New("TOKEN", 0)
	store.Endpoint = server.URL

	if name, err := store.Me(context.Background()); err != nil || name != "my_bot" {
		t.Errorf("Me = %q, %v", name, err)
	}
	chat, heard, err := store.FindChat(context.Background(), false, -1)
	if err != nil || chat.ID != -200 || chat.Title != "files" || heard != 9 {
		t.Errorf("FindChat = %+v, %d, %v, want the newest channel and 9", chat, heard, err)
	}

	store.Token = "WRONG"
	if _, err := store.Me(context.Background()); err == nil {
		t.Error("Me accepted a wrong token")
	}
}
