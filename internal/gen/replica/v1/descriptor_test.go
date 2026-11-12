package v1

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestContractDescriptorDrift(t *testing.T) {
	source, err := os.ReadFile("../../../../contracts/replication.proto")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != "96c8fd050b078b4333f54a75c724f44f87813e42ffea9c0f8ce689e787548097" {
		t.Fatalf("contract hash changed: %s", got)
	}
	expected, err := os.ReadFile("../../../enterprise/pullbudget/replication.pbdesc")
	if err != nil {
		t.Fatal(err)
	}
	file := protodesc.ToFileDescriptorProto(File_replication_proto)
	got, err := proto.Marshal(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatal("generated descriptor differs from pullbudget descriptor")
	}
}
