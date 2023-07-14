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

	// upload model
	fmt.Println("upload model")
	ss, err := client.UploadFile(ctx, "/home/leor/a.txt", "/ip4/192.168.50.191/udp/5154/quic/webtransport/certhash/uEiDE31O-TXq0q-WPrEfxLNMKAQkfQMS732E6j8s8VeHxBw/certhash/uEiDG9-zriMhbz-bHX858bc1WUrcjHCTeyjjLXTToFUN7DQ/p2p/12D3KooWDSuvsdxaKiP9UtAoxAYwWbtJbhyQxx23Aous67LN7h8K")
	if err != nil {
		fmt.Println("upload file err: ", err)
		return
	}
	fmt.Println("cid", ss[0])

	alias, dataId, err := client.CreateFile(ctx, "filename", ss[0], "example", 365, 100, 1, 4)
	if err != nil {
		fmt.Println("err: ", err)
	}
	fmt.Println("alias: ", alias, "dataId: ", dataId)

	fmt.Println("create model...")
	content := "{\"nickname\": \"irene\"}"
	groupId := "example"
	duration := uint64(7)
	delay := uint64(100)
	name := "nickname"
	alias, dataId, err = client.CreateModel(ctx, content, groupId, duration, delay, name, 1, false)
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
