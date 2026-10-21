package grpcv2

import (
	oldpb "auxilia/pb"
	pb "auxilia/pb/v2"
	"bytes"
	"encoding/binary"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLobbyGRPCWebWatchAndLegacyMutation(t *testing.T) {
	f := transport(t)
	c := pb.NewLobbyServiceV2Client(f.conn)
	initial, err := c.CreateLobby(f.contexts[0], &pb.CreateLobbyRequest{RoomName: "web"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(grpcweb.WrapServer(f.server, grpcweb.WithOriginFunc(func(string) bool { return true })))
	defer server.Close()
	raw, err := proto.Marshal(&pb.LobbyRequest{RoomId: initial.RoomId})
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 5+len(raw))
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(raw)))
	copy(frame[5:], raw)
	req, err := http.NewRequestWithContext(f.contexts[0], "POST", server.URL+"/game.network.v2.LobbyServiceV2/WatchRoom", bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(f.contexts[0])
	req.Header.Set("Authorization", md.Get("authorization")[0])
	req.Header.Set("Content-Type", "application/grpc-web+proto")
	req.Header.Set("X-Grpc-Web", "1")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	read := func() *pb.RoomSnapshot {
		t.Helper()
		header := make([]byte, 5)
		if _, err := io.ReadFull(response.Body, header); err != nil {
			t.Fatal(err)
		}
		if header[0] != 0 {
			t.Fatal("expected data frame", header)
		}
		raw := make([]byte, binary.BigEndian.Uint32(header[1:]))
		if _, err := io.ReadFull(response.Body, raw); err != nil {
			t.Fatal(err)
		}
		result := &pb.RoomSnapshot{}
		if err := proto.Unmarshal(raw, result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := read(); got.Revision != initial.Revision {
		t.Fatal(got)
	}
	if _, err := f.rooms.SetReady(f.contexts[0], &oldpb.SetReadyRequest{RoomId: int32(initial.RoomId), UserId: f.ids[0], Ready: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Revision <= initial.Revision || !got.Members[0].Ready {
		t.Fatal("legacy change not streamed", got)
	}
	joined, err := f.rooms.JoinRoom(f.contexts[0], &oldpb.JoinRoomRequest{RoomId: int32(initial.RoomId), UserId: f.ids[0]})
	if err != nil || !joined.Rooms[0].IsReady || joined.Rooms[0].State != 1 {
		t.Fatal(joined, err)
	}
}
