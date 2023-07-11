package main

import (
	"context"
	"fmt"

	"github.com/SaoNetwork/sao-client-go/sdk"
)

func main() {
	ctx := context.Background()

	client, err := sdk.NewSaoClientApi(ctx, "https://gateway-beta.sao.network:443/rpc/v0", "https://rpc-beta.sao.network:443", "leo", "~/.sao")
	if err != nil {
		return
	}

	fmt.Println("create model...")
	content := "{\"nickname\": \"irene\"}"
	groupId := "example"
	duration := uint64(7)
	delay := uint64(100)
	name := "nickname"
	alias, dataId, err := client.CreateModel(ctx, content, groupId, duration, delay, name, 1)
	if err != nil {
		fmt.Println("create model error: ", err)
		return
	}
	fmt.Println("model created alias: ", alias, " data id: ", dataId)

	err = client.UpdatePermission(ctx,
		dataId,
		[]string{"did:key:zQ3shggYEtCZNEiwSeqLdLo97SqS2ERMHB2mgV8hmCGDn4DJ3"},
		[]string{},
	)
	if err != nil {
		fmt.Println("error: ", err)
	} else {
		fmt.Println("Permission Updated")
		// download link
		// https://gateway-beta.sao.network/sao/2ce74379-1fdd-11ee-a875-be229d211050
		// {"nickname": "irene"}
	}

}
