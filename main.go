package main

import (
	"context"
	"fmt"

	"github.com/SaoNetwork/sao-client-go/sdk"
)

func main() {
	ctx := context.Background()

	client, err := sdk.NewSaoClientApi(ctx, "http://127.0.0.1:5151/rpc/v0", "http://127.0.0.1:26657", "leo")
	if err != nil {
		return
	}

	fmt.Println("create model...")
	content := "content2"
	groupId := "groupId"
	duration := uint64(10)
	delay := uint64(10)
	name := "name2"
	alias, dataId, err := client.CreateModel(ctx, content, groupId, duration, delay, name, 1)
	if err != nil {
		fmt.Println("create model error: ", err)
		return
	}
	fmt.Println("model created alias: ", alias, " data id: ", dataId)

	// alias := "name2"
	// dataId := "8d06fc3c-1fc1-11ee-8558-b766aa48891e"

	fmt.Println("load model...")
	bytes, err := client.Load(ctx, dataId, "", "", groupId)
	if err != nil {
		fmt.Println("load model error: ", err)
		return
	}
	fmt.Println("load model: ", string(bytes))

	resp, err := client.ShowCommits(ctx, dataId, groupId)
	if err != nil {
		fmt.Println("show commits error: ", err)
		return
	}
	fmt.Println("alias: ", resp.Alias)
	fmt.Println("dataId: ", resp.DataId)
	fmt.Println("Commits: ")
	for _, c := range resp.Commits {
		fmt.Println(c)
	}

	err = client.UpdatePermission(ctx, dataId,
		[]string{"did:key:zQ3shggYEtCZNEiwSeqLdLo97SqS2ERMHB2mgV8hmCGDn4DJ3"},
		[]string{"did:key:zQ3shggYEtCZNEiwSeqLdLo97SqS2ERMHB2mgV8hmCGDn4DJ3"},
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Permission Updated.")

	patch, cid, contentLen, err := client.PatchGen("{\"a\": \"b\"}", "{\"a\":\"c\"}")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("patch: ", patch)
	fmt.Println("cid: ", cid)
	fmt.Println("content len: ", contentLen)
}
