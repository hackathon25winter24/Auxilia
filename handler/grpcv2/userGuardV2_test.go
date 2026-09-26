package grpcv2

import (
	"auxilia/pb"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestProfileCannotBypassAuthoritativeRatings(t *testing.T) {
	f := transport(t)
	client := pb.NewUserServiceClient(f.conn)
	request := &pb.UpdateUserRequest{Id: f.ids[0], Name: "updated", Rate: 99999, NumWins: 999, NumBattles: 999}
	if _, err := client.UpdateUser(context.Background(), request); status.Code(err) != codes.Unauthenticated {
		t.Fatal("anonymous profile write", err)
	}
	if _, err := client.UpdateUser(f.contexts[1], request); status.Code(err) != codes.PermissionDenied {
		t.Fatal("other profile write", err)
	}
	got, err := client.UpdateUser(f.contexts[0], request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rate != 1500 || got.NumWins != 0 || got.NumBattles != 0 || got.Name != "updated" {
		t.Fatal("client overwrote server counters")
	}
	_ = startRPC(t, f)
	if _, err := client.DeleteUser(f.contexts[0], &pb.DeleteUserRequest{Id: f.ids[0]}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("participant removed during battle", err)
	}
}
