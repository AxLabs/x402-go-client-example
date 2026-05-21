// Package permit2 ensures ERC-20 allowance to the canonical Permit2 contract
// before x402 payments that use the permit2 transfer method.
package permit2

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
	x402evm "github.com/x402-foundation/x402/go/mechanisms/evm"
	exactclient "github.com/x402-foundation/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/x402-foundation/x402/go/signers/evm"
)

// RequiresPermit2 reports whether the payment option uses Permit2 (not EIP-3009).
func RequiresPermit2(req x402adapter.Requirements) bool {
	if req.Extra != nil {
		if raw, ok := req.Extra["assetTransferMethod"]; ok {
			if method, ok := raw.(string); ok {
				return strings.EqualFold(method, "permit2")
			}
		}
	}
	return false
}

// Preparer broadcasts an on-chain approve(Permit2, max) when allowance is low.
type Preparer struct {
	privateKeyHex string
	rpcURL        func(network string) string
}

// NewPreparer returns a preparer that resolves RPC URLs per network.
// rpcURL must return a non-empty HTTP RPC endpoint for EVM networks.
func NewPreparer(privateKeyHex string, rpcURL func(network string) string) *Preparer {
	return &Preparer{
		privateKeyHex: privateKeyHex,
		rpcURL:        rpcURL,
	}
}

// EnsureAllowance checks token.allowance(owner, Permit2) and, if below the
// payment amount, signs and broadcasts approve(Permit2, maxUint256).
func (p *Preparer) EnsureAllowance(ctx context.Context, req x402adapter.Requirements) error {
	if p == nil || p.privateKeyHex == "" {
		return fmt.Errorf("permit2 preparer: private key not configured")
	}
	if !RequiresPermit2(req) {
		return nil
	}

	rpc := p.rpcURL(string(req.Network))
	if rpc == "" {
		return fmt.Errorf("permit2 preparer: no RPC URL for network %s (set CLIENT_RPC_URL or CLIENT_RPC_%s)",
			req.Network, envKeyForNetwork(string(req.Network)))
	}

	required, ok := new(big.Int).SetString(req.Amount, 10)
	if !ok {
		return fmt.Errorf("permit2 preparer: invalid payment amount %q", req.Amount)
	}

	ethClient, err := ethclient.DialContext(ctx, rpc)
	if err != nil {
		return fmt.Errorf("permit2 preparer: connect RPC: %w", err)
	}
	defer ethClient.Close()

	signer, err := evmsigners.NewClientSignerFromPrivateKeyWithClient(p.privateKeyHex, ethClient)
	if err != nil {
		return fmt.Errorf("permit2 preparer: signer: %w", err)
	}

	readSigner, ok := signer.(x402evm.ClientEvmSignerWithReadContract)
	if !ok {
		return fmt.Errorf("permit2 preparer: signer does not support ReadContract")
	}

	token := x402evm.NormalizeAddress(req.Asset)
	owner := common.HexToAddress(signer.Address())

	allowanceResult, err := readSigner.ReadContract(
		ctx,
		token,
		x402evm.ERC20AllowanceABI,
		"allowance",
		owner,
		common.HexToAddress(x402evm.PERMIT2Address),
	)
	if err != nil {
		return fmt.Errorf("permit2 preparer: read allowance: %w", err)
	}
	allowance, ok := allowanceResult.(*big.Int)
	if !ok {
		return fmt.Errorf("permit2 preparer: unexpected allowance type %T", allowanceResult)
	}
	if allowance.Cmp(required) >= 0 {
		return nil
	}

	chainID, err := chainIDFromNetwork(string(req.Network))
	if err != nil {
		return err
	}
	if err := p.broadcastApprove(ctx, ethClient, signer, token, chainID); err != nil {
		return err
	}

	// Re-read after mining; some RPCs can return a receipt before eth_call sees new allowance.
	return p.waitForAllowance(ctx, readSigner, token, owner, required)
}

func chainIDFromNetwork(network string) (*big.Int, error) {
	chainID, err := x402evm.GetEvmChainId(network)
	if err != nil {
		return nil, fmt.Errorf("permit2 preparer: chain id: %w", err)
	}
	return chainID, nil
}

func (p *Preparer) readAllowance(
	ctx context.Context,
	readSigner x402evm.ClientEvmSignerWithReadContract,
	token string,
	owner common.Address,
) (*big.Int, error) {
	allowanceResult, err := readSigner.ReadContract(
		ctx,
		token,
		x402evm.ERC20AllowanceABI,
		"allowance",
		owner,
		common.HexToAddress(x402evm.PERMIT2Address),
	)
	if err != nil {
		return nil, fmt.Errorf("read allowance: %w", err)
	}
	allowance, ok := allowanceResult.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected allowance type %T", allowanceResult)
	}
	return allowance, nil
}

func (p *Preparer) waitForAllowance(
	ctx context.Context,
	readSigner x402evm.ClientEvmSignerWithReadContract,
	token string,
	owner common.Address,
	required *big.Int,
) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		allowance, err := p.readAllowance(ctx, readSigner, token, owner)
		if err != nil {
			return fmt.Errorf("permit2 preparer: %w", err)
		}
		if allowance.Cmp(required) >= 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("permit2 preparer: allowance still below required after approve (need %s)", required.String())
}

func (p *Preparer) broadcastApprove(
	ctx context.Context,
	ethClient *ethclient.Client,
	signer x402evm.ClientEvmSigner,
	token string,
	chainID *big.Int,
) error {
	txSigner, ok := signer.(x402evm.ClientEvmSignerWithTxSigning)
	if !ok {
		return fmt.Errorf("permit2 preparer: signer does not support transaction signing")
	}

	info, err := exactclient.SignErc20ApprovalTransaction(ctx, txSigner, token, chainID)
	if err != nil {
		return fmt.Errorf("permit2 preparer: sign approve: %w", err)
	}

	raw, err := hex.DecodeString(strings.TrimPrefix(info.SignedTransaction, "0x"))
	if err != nil {
		return fmt.Errorf("permit2 preparer: decode signed tx: %w", err)
	}
	var signedTx types.Transaction
	if err := signedTx.UnmarshalBinary(raw); err != nil {
		return fmt.Errorf("permit2 preparer: unmarshal signed tx: %w", err)
	}

	if err := ethClient.SendTransaction(ctx, &signedTx); err != nil {
		return fmt.Errorf("permit2 preparer: broadcast approve: %w", err)
	}

	receiptCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	receipt, err := ethClient.TransactionReceipt(receiptCtx, signedTx.Hash())
	if err != nil {
		return fmt.Errorf("permit2 preparer: wait for approve tx %s: %w", signedTx.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("permit2 preparer: approve tx %s failed on-chain", signedTx.Hash().Hex())
	}

	return nil
}

// envKeyForNetwork builds the per-network env var suffix, e.g. CLIENT_RPC_EIP155_84532.
func envKeyForNetwork(network string) string {
	return "CLIENT_RPC_" + strings.ToUpper(strings.ReplaceAll(network, ":", "_"))
}
