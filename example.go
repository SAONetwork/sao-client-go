package main

import (
	"context"
	"fmt"

	"github.com/SaoNetwork/sao-client-go/sdk"
)

func main() {
	ctx := context.Background()

	client, err := sdk.NewSaoClientApi(
		ctx,
		"https://gateway-beta.sao.network:443/rpc/v0", // gateway api
		"https://rpc-beta.sao.network:443",            // chain api
		"irene",                                       // account key name
		"~/.sao",                                      // keyring home dir
	)
	if err != nil {
		return
	}

	// upload model
	fmt.Println("upload model")
	ss, err := client.UploadFile(
		ctx,
		// local file path
		"foo.txt",
		// multiaddr of gateway
		"/ip4/8.222.225.178/udp/5154/quic/webtransport/certhash/uEiAEe-50if6gVaECe0NKhKBhHEMySfy4HtAD2VexGODPaA/certhash/uEiDhqfDJEUnPGh9BMCzoWVTKpA4V3aunIf7F1fCgi1rA5A/p2p/12D3KooWJA2R7RTd6aD2pUdvjN29FdiC8f5edSifXA2tXBcbA2UX",
	)
	if err != nil {
		fmt.Println("upload file err: ", err)
		return
	}
	fmt.Println("cid", ss[0])

	alias, dataId, err := client.CreateFile(
		ctx,
		"foo.txt", // file name
		ss[0],     // cid
		"example", // group id
		365,       // duration days
		100,       // delay epochs
		1,         // replica count
		4,         // file size of foo.txt
	)
	if err != nil {
		fmt.Println("err: ", err)
		return
	}
	fmt.Println("alias: ", alias, "dataId: ", dataId)
	// download link
	// https://gateway-beta.sao.network/sao/2ce74379-1fdd-11ee-a875-be229d211050
	// {"nickname": "irene"}

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
