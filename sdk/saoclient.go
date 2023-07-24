package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	did "github.com/SaoNetwork/sao-did"
	saodid "github.com/SaoNetwork/sao-did"
	saokey "github.com/SaoNetwork/sao-did/key"
	api "github.com/SaoNetwork/sao-node/api"
	apitypes "github.com/SaoNetwork/sao-node/api/types"
	"github.com/SaoNetwork/sao-node/chain"
	saoclient "github.com/SaoNetwork/sao-node/client"
	types "github.com/SaoNetwork/sao-node/types"
	utils "github.com/SaoNetwork/sao-node/utils"
	saotypes "github.com/SaoNetwork/sao/x/sao/types"
	"github.com/filecoin-project/go-jsonrpc"
	cid "github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

func NewNodeApi(ctx context.Context, address string, token string) (api.SaoApi, jsonrpc.ClientCloser, error) {
	var res api.SaoApiStruct

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))

	closer, err := jsonrpc.NewMergeClient(
		ctx,
		address,
		"Sao",
		api.GetInternalStructs(&res),
		headers,
	)
	return &res, closer, err
}

type SaoClientApi struct {
	client        *SaoClient
	NodeEndpoint  string
	ChainEndpoint string
	Closer        func()
	keyName       string
	keyringHome   string
}

func NewSaoClientApi(ctx context.Context, nodeEndpoint string, chainEndpoint string, KeyName string, keyringHome string) (*SaoClientApi, error) {
	client, closer, err := NewSaoClient(ctx, nodeEndpoint, chainEndpoint)
	if err != nil {
		return nil, err
	}

	return &SaoClientApi{
		NodeEndpoint:  nodeEndpoint,
		ChainEndpoint: chainEndpoint,
		Closer:        closer,
		client:        client,
		keyName:       KeyName,
		keyringHome:   keyringHome,
	}, nil
}

type SaoClient struct {
	api.SaoApi
	chain.ChainSvcApi
}

func NewSaoClient(ctx context.Context, nodeEndpoint string, chainEndpoint string) (*SaoClient, func(), error) {
	var gatewayApi api.SaoApi = nil
	var closer = func() {}
	var err error
	gatewayApi, closer, err = NewNodeApi(ctx, nodeEndpoint, "default token")
	if err != nil {
		return nil, nil, err
	}
	chainSvc, err := chain.NewChainSvc(ctx, chainEndpoint, "/websocket", "~/.sao")
	if err != nil {
		return nil, nil, err
	}
	return &SaoClient{
		SaoApi:      gatewayApi,
		ChainSvcApi: chainSvc,
	}, closer, nil
}

func (sc *SaoClientApi) GetDidManager(ctx context.Context, keyName string) (*saodid.DidManager, string, error) {
	address, err := chain.GetAddress(ctx, sc.keyringHome, keyName)
	if err != nil {
		return nil, "", err
	}

	payload := fmt.Sprintf("cosmos %s allows to generate did", address)
	secret, err := chain.SignByAccount(ctx, sc.keyringHome, keyName, []byte(payload))
	if err != nil {
		return nil, "", types.Wrap(types.ErrSignedFailed, err)
	}

	provider, err := saokey.NewSecp256k1Provider(secret)
	if err != nil {
		return nil, "", types.Wrap(types.ErrCreateProviderFailed, err)
	}
	resolver := saokey.NewKeyResolver()

	didManager := saodid.NewDidManager(provider, resolver)
	_, err = didManager.Authenticate([]string{}, "")
	if err != nil {
		return nil, "", types.Wrap(types.ErrAuthenticateFailed, err)
	}

	return &didManager, address, nil
}

func (sc *SaoClientApi) Renew(
	ctx context.Context,
	dataIds []string,
	duration uint64,
	delay uint64,
) (map[string]uint64, map[string]string, map[string]string, error) {
	if len(dataIds) <= 0 {
		return nil, nil, nil, xerrors.Errorf("data ids is missing.")
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to get did manager %v", err)
	}

	proposal := saotypes.RenewProposal{
		Owner:    didManager.Id,
		Duration: uint64(time.Duration(60*60*24*duration) * time.Second / chain.Blocktime),
		Timeout:  int32(delay),
		Data:     dataIds,
	}

	proposalBytes, err := proposal.Marshal()
	if err != nil {
		return nil, nil, nil, types.Wrap(types.ErrMarshalFailed, err)
	}

	jws, err := didManager.CreateJWS(proposalBytes)
	if err != nil {
		return nil, nil, nil, types.Wrap(types.ErrCreateJwsFailed, err)
	}
	clientProposal := types.OrderRenewProposal{
		Proposal:     proposal,
		JwsSignature: saotypes.JwsSignature(jws.Signatures[0]),
	}

	var results map[string]string
	res, err := sc.client.ModelRenewOrder(ctx, &clientProposal, true)
	if err != nil {
		return nil, nil, nil, err
	}
	results = res.Results

	var renewModels = make(map[string]uint64, len(results))
	var renewedOrders = make(map[string]string, 0)
	var failedOrders = make(map[string]string, 0)
	for dataId, result := range results {
		if strings.Contains(result, "SUCCESS") {
			orderId, err := strconv.ParseUint(strings.Split(result, "=")[1], 10, 64)
			if err != nil {
				failedOrders[dataId] = result + ", " + err.Error()
			} else {
				renewModels[dataId] = orderId
			}
		} else {
			renewedOrders[dataId] = result
		}
	}
	return renewModels, renewedOrders, failedOrders, nil
}

func (sc *SaoClientApi) ShowCommits(
	ctx context.Context,
	keyword string,
	groupId string,
) (*apitypes.ShowCommitsResp, error) {
	if keyword == "" {
		return nil, xerrors.Errorf("keyword is missing.")
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return nil, xerrors.Errorf("failed to get did manager %v", err)
	}

	proposal := saotypes.QueryProposal{
		Owner:   didManager.Id,
		Keyword: keyword,
		GroupId: groupId,
	}

	if !utils.IsDataId(keyword) {
		proposal.KeywordType = 2
	}

	gatewayAddress, err := sc.client.GetNodeAddress(ctx)
	if err != nil {
		return nil, err
	}

	request, err := sc.buildQueryRequest(ctx, didManager, proposal, sc.client, gatewayAddress)
	if err != nil {
		return nil, err
	}

	resp, err := sc.client.ModelShowCommits(ctx, request)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (sc *SaoClientApi) Delete(
	ctx context.Context,
	dataId string,
) (string, error) {
	if dataId == "" {
		return "", xerrors.Errorf("dataId is missing")
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return "", xerrors.Errorf("failed to get did manager %v", err)
	}

	proposal := saotypes.TerminateProposal{
		Owner:  didManager.Id,
		DataId: dataId,
	}

	proposalBytes, err := proposal.Marshal()
	if err != nil {
		return "", types.Wrap(types.ErrMarshalFailed, err)
	}

	jws, err := didManager.CreateJWS(proposalBytes)
	if err != nil {
		return "", types.Wrap(types.ErrCreateJwsFailed, err)
	}
	request := types.OrderTerminateProposal{
		Proposal:     proposal,
		JwsSignature: saotypes.JwsSignature(jws.Signatures[0]),
	}

	result, err := sc.client.ModelDelete(ctx, &request, true)
	if err != nil {
		return "", err
	}
	return result.DataId, err
}

func (sc *SaoClientApi) Load(
	ctx context.Context,
	keyword string,
	version string,
	commitId string,
	groupId string,
) ([]byte, error) {
	if keyword == "" {
		return nil, xerrors.Errorf("keyword is missing")
	}
	if version != "" && commitId != "" {
		version = ""
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return nil, xerrors.Errorf("failed to get did manager %v", err)
	}

	proposal := saotypes.QueryProposal{
		Owner:    didManager.Id,
		Keyword:  keyword,
		GroupId:  groupId,
		CommitId: commitId,
		Version:  version,
	}

	if !utils.IsDataId(keyword) {
		proposal.KeywordType = 2
	}

	gatewayAddress, err := sc.client.GetNodeAddress(ctx)
	if err != nil {
		return nil, err
	}

	request, err := sc.buildQueryRequest(ctx, didManager, proposal, sc.client, gatewayAddress)
	if err != nil {
		return nil, err
	}

	resp, err := sc.client.ModelLoad(ctx, request)
	if err != nil {
		return nil, err
	}
	return []byte(resp.Content), nil
}

func (sc *SaoClientApi) UpdatePermission(
	ctx context.Context,
	dataId string,
	readonlyDids []string,
	readwriteDids []string,
) error {
	if dataId == "" {
		return xerrors.Errorf("data id is missing")
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return xerrors.Errorf("failed to get did manager %v", err)
	}

	proposal := saotypes.PermissionProposal{
		Owner:         didManager.Id,
		DataId:        dataId,
		ReadonlyDids:  readonlyDids,
		ReadwriteDids: readwriteDids,
	}

	proposalBytes, err := proposal.Marshal()
	if err != nil {
		return types.Wrap(types.ErrMarshalFailed, err)
	}

	jws, err := didManager.CreateJWS(proposalBytes)
	if err != nil {
		return types.Wrap(types.ErrCreateJwsFailed, err)
	}

	request := &types.PermissionProposal{
		Proposal: proposal,
		JwsSignature: saotypes.JwsSignature{
			Protected: jws.Signatures[0].Protected,
			Signature: jws.Signatures[0].Signature,
		},
	}

	_, err = sc.client.ModelUpdatePermission(ctx, request, true)
	if err != nil {
		return err
	}
	return nil
}

func (sc *SaoClientApi) PatchGen(
	origin string,
	target string,
) (string, cid.Cid, int, error) {
	patch, err := utils.GeneratePatch(origin, target)
	if err != nil {
		return "", cid.Undef, 0, err
	}

	content, err := utils.ApplyPatch([]byte(origin), []byte(patch))
	if err != nil {
		return "", cid.Undef, 0, err
	}

	var newModel interface{}
	err = json.Unmarshal(content, &newModel)
	if err != nil {
		return "", cid.Undef, 0, err
	}

	var targetModel interface{}
	err = json.Unmarshal([]byte(target), &targetModel)
	if err != nil {
		return "", cid.Undef, 0, err
	}

	valueStrNew, err := json.Marshal(newModel)
	if err != nil {
		return "", cid.Undef, 0, err
	}

	valueStrTarget, err := json.Marshal(targetModel)
	if err != nil {
		return "", cid.Undef, 0, err
	}

	if string(valueStrNew) != string(valueStrTarget) {
		return "", cid.Undef, 0, err
	}

	targetCid, err := utils.CalculateCid(content)
	if err != nil {
		return "", cid.Undef, 0, err
	}

	return patch, targetCid, len(content), nil
}

func (sc *SaoClientApi) UpdateModel(
	ctx context.Context,
	patch string,
	duration uint64,
	delay uint64,
	force bool,
	keyword string,
	commitId string,
	cidstring string,
	size uint64,
	replica uint64,
	groupId string,
) (string, string, string, error) {
	if keyword == "" {
		return "", "", "", xerrors.Errorf("must provide keyword.")
	}
	if size == 0 {
		return "", "", "", xerrors.Errorf("invalid size")
	}
	newCid, err := cid.Decode(cidstring)
	if err != nil {
		return "", "", "", xerrors.Errorf("invalid cid: %v", cidstring)
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return "", "", "", xerrors.Errorf("failed to get did manager %v", err)
	}

	gatewayAddress, err := sc.client.GetNodeAddress(ctx)
	if err != nil {
		return "", "", "", err
	}

	queryProposal := saotypes.QueryProposal{
		Owner:   didManager.Id,
		Keyword: keyword,
		GroupId: groupId,
	}

	if !utils.IsDataId(keyword) {
		queryProposal.KeywordType = 2
	}

	request, err := sc.buildQueryRequest(ctx, didManager, queryProposal, sc.client, gatewayAddress)
	if err != nil {
		return "", "", "", err
	}

	res, err := sc.client.QueryMetadata(ctx, request, 0)
	if err != nil {
		return "", "", "", err
	}

	operation := uint32(1)

	if force {
		operation = 2
	}

	proposal := saotypes.Proposal{
		Owner:      didManager.Id,
		Provider:   gatewayAddress,
		GroupId:    groupId,
		Duration:   uint64(time.Duration(60*60*24*duration) * time.Second / chain.Blocktime),
		Replica:    int32(replica),
		Timeout:    int32(delay),
		DataId:     res.Metadata.DataId,
		Alias:      res.Metadata.Alias,
		Tags:       []string{},
		Cid:        newCid.String(),
		CommitId:   commitId + "|" + utils.GenerateCommitId(didManager.Id+groupId),
		Rule:       "",
		Operation:  operation,
		Size_:      uint64(size),
		ExtendInfo: "",
	}

	clientProposal, err := sc.buildClientProposal(ctx, didManager, proposal, sc.client)
	if err != nil {
		return "", "", "", err
	}

	resp, err := sc.client.ModelUpdate(ctx, request, clientProposal, 0, []byte(patch))
	if err != nil {
		return "", "", "", err
	}
	return resp.Alias, resp.DataId, resp.CommitId, nil
}

func (sc *SaoClientApi) UploadFile(
	ctx context.Context,
	fpath string,
	multiaddr string,
) ([]string, error) {
	if !strings.Contains(multiaddr, "/p2p/") {
		return nil, types.Wrapf(types.ErrInvalidParameters, "invalid multiaddr: %s", multiaddr)
	}
	peerId := strings.Split(multiaddr, "/p2p/")[1]

	var files []string
	err := filepath.Walk(fpath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			files = append(files, path)
		} else {
			fmt.Printf("skip directory %s\r\n", path)
		}

		return nil
	})

	if err != nil {
		return nil, types.Wrap(types.ErrInvalidParameters, err)
	}

	var cids []string
	for _, file := range files {
		c := saoclient.DoTransport(ctx, "~/.sao-cli", multiaddr, peerId, file)
		if c != cid.Undef {
			cids = append(cids, c.String())
		} else {
			return nil, xerrors.Errorf("upload error")
		}
	}
	return cids, nil
}

func (sc *SaoClientApi) CreateFile(
	ctx context.Context,
	fileName string,
	cidString string,
	groupId string,
	duration uint64,
	delay uint64,
	replicas uint64,
	size uint64,
) (string, string, error) {
	if fileName == "" {
		return "", "", xerrors.Errorf("must provide file name")
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return "", "", xerrors.Errorf("failed to get did manager %v", err)
	}

	gatewayAddress, err := sc.client.GetNodeAddress(ctx)
	if err != nil {
		return "", "", err
	}

	contentCid, err := cid.Decode(cidString)
	if err != nil {
		return "", "", types.Wrap(types.ErrInvalidCid, err)
	}

	dataId := utils.GenerateDataId(didManager.Id + groupId)
	proposal := saotypes.Proposal{
		DataId:     dataId,
		Owner:      didManager.Id,
		Provider:   gatewayAddress,
		GroupId:    groupId,
		Duration:   uint64(time.Duration(60*60*24*duration) * time.Second / chain.Blocktime),
		Replica:    int32(replicas),
		Timeout:    int32(delay),
		Alias:      fileName,
		Tags:       []string{},
		Cid:        contentCid.String(),
		CommitId:   dataId,
		Rule:       "",
		Operation:  1,
		ExtendInfo: "",
		Size_:      size,
	}

	clientProposal, err := sc.buildClientProposal(ctx, didManager, proposal, sc.client)
	if err != nil {
		return "", "", err
	}

	var orderId uint64 = 0

	queryProposal := saotypes.QueryProposal{
		Owner:   didManager.Id,
		Keyword: dataId,
	}

	request, err := sc.buildQueryRequest(ctx, didManager, queryProposal, sc.client, gatewayAddress)
	if err != nil {
		return "", "", err
	}

	resp, err := sc.client.ModelCreateFile(ctx, request, clientProposal, orderId)
	if err != nil {
		return "", "", err
	}
	return resp.Alias, resp.DataId, nil
}

func (sc *SaoClientApi) CreateModel(
	ctx context.Context,
	content string,
	groupId string,
	duration uint64,
	delay uint64,
	name string,
	replicas uint64,
	isPublic bool,
) (string, string, error) {
	if content == "" {
		return "", "", xerrors.Errorf("must provide content")
	}

	contentBytes := []byte(content)
	contentCid, err := CalculateCid(contentBytes)
	if err != nil {
		return "", "", err
	}

	didManager, _, err := sc.GetDidManager(ctx, sc.keyName)
	if err != nil {
		return "", "", xerrors.Errorf("failed to get did manager %v", err)
	}

	gatewayAddress, err := sc.client.GetNodeAddress(ctx)
	if err != nil {
		return "", "", err
	}

	dataId := utils.GenerateDataId(didManager.Id + groupId)
	proposal := saotypes.Proposal{
		DataId:     dataId,
		Owner:      didManager.Id,
		Provider:   gatewayAddress,
		GroupId:    groupId,
		Duration:   uint64(time.Duration(60*60*24*duration) * time.Second / chain.Blocktime),
		Replica:    int32(replicas),
		Timeout:    int32(delay),
		Alias:      name,
		Tags:       []string{""},
		Cid:        contentCid.String(),
		CommitId:   dataId,
		Rule:       "",
		Size_:      uint64(len(content)),
		Operation:  1,
		ExtendInfo: "",
	}
	if proposal.Alias == "" {
		proposal.Alias = proposal.Cid
	}
	queryProposal := saotypes.QueryProposal{
		Owner:   didManager.Id,
		Keyword: dataId,
	}
	clientProposal, err := sc.buildClientProposal(ctx, didManager, proposal, sc.client)
	if err != nil {
		return "", "", err
	}

	request, err := sc.buildQueryRequest(ctx, didManager, queryProposal, sc.client, gatewayAddress)
	if err != nil {
		return "", "", err
	}

	resp, err := sc.client.ModelCreate(ctx, request, clientProposal, 0, contentBytes)
	if err != nil {
		return "", "", err
	}

	if isPublic {
		//builtinDids, err := sc.client.QueryDidParams(ctx)
		//if err != nil {
		//	return "", "", err
		//}
		//
		//proposal := saotypes.PermissionProposal{
		//	Owner:         didManager.Id,
		//	DataId:        resp.DataId,
		//	ReadonlyDids:  strings.Split(builtinDids, ","),
		//	ReadwriteDids: []string{},
		//}
		//
		//proposalBytes, err := proposal.Marshal()
		//if err != nil {
		//	return "", "", types.Wrap(types.ErrMarshalFailed, err)
		//}
		//
		//jws, err := didManager.CreateJWS(proposalBytes)
		//if err != nil {
		//	return "", "", types.Wrap(types.ErrCreateJwsFailed, err)
		//}
		//
		//request := &types.PermissionProposal{
		//	Proposal: proposal,
		//	JwsSignature: saotypes.JwsSignature{
		//		Protected: jws.Signatures[0].Protected,
		//		Signature: jws.Signatures[0].Signature,
		//	},
		//}
		//
		//_, err = sc.client.ModelUpdatePermission(ctx, request, true)
		//if err != nil {
		//	return "", "", err
		//}
	}


	return resp.Alias, resp.DataId, nil
}

func (sc *SaoClientApi) buildQueryRequest(
	ctx context.Context,
	didManager *did.DidManager,
	proposal saotypes.QueryProposal,
	chain chain.ChainSvcApi,
	gatewayAddress string,
) (*types.MetadataProposal, error) {
	lastHeight, err := chain.GetLastHeight(ctx)
	if err != nil {
		return nil, types.Wrap(types.ErrQueryHeightFailed, err)
	}

	peerInfo, err := chain.GetNodePeer(ctx, gatewayAddress)
	if err != nil {
		return nil, err
	}

	proposal.LastValidHeight = uint64(lastHeight + 200)
	proposal.Gateway = peerInfo

	if proposal.Owner == "all" {
		return &types.MetadataProposal{
			Proposal: proposal,
		}, nil
	}

	proposalBytes, err := proposal.Marshal()
	if err != nil {
		return nil, types.Wrap(types.ErrMarshalFailed, err)
	}

	jws, err := didManager.CreateJWS(proposalBytes)
	if err != nil {
		return nil, types.Wrap(types.ErrCreateJwsFailed, err)
	}

	return &types.MetadataProposal{
		Proposal: proposal,
		JwsSignature: saotypes.JwsSignature{
			Protected: jws.Signatures[0].Protected,
			Signature: jws.Signatures[0].Signature,
		},
	}, nil
}

func (sc *SaoClientApi) buildClientProposal(_ context.Context, didManager *did.DidManager, proposal saotypes.Proposal, _ chain.ChainSvcApi) (*types.OrderStoreProposal, error) {
	proposalBytes, err := proposal.Marshal()
	if err != nil {
		return nil, types.Wrap(types.ErrMarshalFailed, err)
	}

	jws, err := didManager.CreateJWS(proposalBytes)
	if err != nil {
		return nil, types.Wrap(types.ErrCreateJwsFailed, err)
	}
	return &types.OrderStoreProposal{
		Proposal: proposal,
		JwsSignature: saotypes.JwsSignature{
			Protected: jws.Signatures[0].Protected,
			Signature: jws.Signatures[0].Signature,
		},
	}, nil
}

func CalculateCid(content []byte) (cid.Cid, error) {
	pref := cid.Prefix{
		Version:  0,
		Codec:    uint64(multicodec.Raw),
		MhType:   multihash.SHA2_256,
		MhLength: -1, // default length
	}

	contentCid, err := pref.Sum(content)
	if err != nil {
		return cid.Undef, xerrors.Errorf("")
	}

	return contentCid, nil
}
