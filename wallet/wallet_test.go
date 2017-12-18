package wallet

import (
	"fmt"
	"testing"
	"time"

	"code.aliyun.com/chain33/chain33/account"
	"code.aliyun.com/chain33/chain33/common"
	"code.aliyun.com/chain33/chain33/common/crypto"
	"code.aliyun.com/chain33/chain33/queue"
	"code.aliyun.com/chain33/chain33/store"
	"code.aliyun.com/chain33/chain33/types"
	"code.aliyun.com/chain33/chain33/util"
)

func init() {
	queue.DisableLog()
}

func initEnv() (*Wallet, *queue.Queue) {
	var q = queue.New("channel")
	var cfg types.Wallet
	cfg.DbPath = "datadir"
	cfg.MinFee = 1000000

	wallet := New(&cfg)
	wallet.SetQueue(q)
	return wallet, q
}

func storeModProc(q *queue.Queue) *store.Store {
	//store
	var cfg types.Store
	cfg.DbPath = "datadir"
	cfg.Driver = "leveldb"
	s := store.New(&cfg)
	s.SetQueue(q)
	return s
}

var Statehash []byte
var CutHeight int64 = 0
var FromAddr string = ""

func blockchainModProc(q *queue.Queue) {
	//store
	go func() {
		client := q.GetClient()
		client.Sub("blockchain")
		for msg := range client.Recv() {
			//walletlog.Info("blockchain", "msg.Ty", msg.Ty)
			if msg.Ty == types.EventGetLastHeader {
				header := &types.Header{StateHash: Statehash}
				msg.Reply(client.NewMessage("account", types.EventHeader, header))
			} else if msg.Ty == types.EventGetTransactionByAddr {
				addr := (msg.Data).(*types.ReqAddr)

				var replyTxInfos types.ReplyTxInfos
				total := 10
				replyTxInfos.TxInfos = make([]*types.ReplyTxInfo, total)

				for index := 0; index < total; index++ {
					var replyTxInfo types.ReplyTxInfo
					hashstr := fmt.Sprintf("hash:%s:%d", addr.Addr, index)
					replyTxInfo.Hash = []byte(hashstr)
					replyTxInfo.Height = CutHeight + 1
					replyTxInfo.Index = int64(index)
					replyTxInfos.TxInfos[index] = &replyTxInfo
					CutHeight += 1
				}
				msg.Reply(client.NewMessage("rpc", types.EventReplyTxInfo, &replyTxInfos))

			} else if msg.Ty == types.EventGetTransactionByHash {
				txhashs := (msg.Data).(*types.ReqHashes)

				var txDetails types.TransactionDetails
				txDetails.Txs = make([]*types.TransactionDetail, len(txhashs.Hashes))
				for index, txhash := range txhashs.Hashes {
					var txDetail types.TransactionDetail
					txDetail.Receipt = &types.ReceiptData{Ty: 2, Logs: nil}
					txDetail.Tx = &types.Transaction{Execer: []byte("coins"), Payload: txhash, To: "14ZTV2wHG3uPHnA5cBJmNxAxxvbzS7Z5mE"}
					txDetails.Txs[index] = &txDetail
				}
				msg.Reply(client.NewMessage("rpc", types.EventTransactionDetails, &txDetails))
			}
		}
	}()
}

func mempoolModProc(q *queue.Queue) {
	//store
	go func() {
		client := q.GetClient()
		client.Sub("mempool")
		for msg := range client.Recv() {
			//walletlog.Info("mempool", "msg.Ty", msg.Ty)
			if msg.Ty == types.EventTx {
				msg.Reply(client.NewMessage("wallet", types.EventReply, &types.Reply{true, nil}))
			}
		}
	}()
}

func SaveAccountTomavl(q *queue.Queue, prevStateRoot []byte, accs []*types.Account) []byte {
	var kvset []*types.KeyValue

	for _, acc := range accs {
		kvs := account.GetKVSet(acc)
		for _, kv := range kvs {
			kvset = append(kvset, kv)
		}

	}
	hash := util.ExecKVSet(q, prevStateRoot, kvset)
	Statehash = hash
	return hash
}
func TestProcCreatNewAccount(t *testing.T) {
	walletlog.Info("TestProcCreatNewAccount begin --------------------")
	wallet, q := initEnv()

	//启动store,blockchain模块
	stor := storeModProc(q)
	blockchainModProc(q)

	total := 10

	addres := make([]string, total)
	accs := make([]*types.Account, total+1)
	for i := 0; i < total; i++ {
		var ReqNewAccount types.ReqNewAccount
		ReqNewAccount.Label = fmt.Sprintf("hybaccount:%d", i)
		time.Sleep(time.Second * 1)
		Walletacc, err := wallet.ProcCreatNewAccount(&ReqNewAccount)
		if err != nil {
			walletlog.Info("ProcCreatNewAccount", "err", err)
			wallet.Close()
			stor.Close()
			return
		}
		addres[i] = Walletacc.Acc.Addr

		Walletacc.Acc.Balance = int64(i)
		Walletacc.Acc.Currency = int32(i)
		Walletacc.Acc.Frozen = int64(i)
		accs[i] = Walletacc.Acc
		FromAddr = Walletacc.Acc.Addr
		walletlog.Info("ProcCreatNewAccount:", "Walletacc", Walletacc.String())
	}

	//通过privkey生成一个pubkey然后换算成对应的addr
	cr, err := crypto.New(types.GetSignatureTypeName(types.SECP256K1))
	if err != nil {
		walletlog.Error("ProcImportPrivKey", "err", err)
	}
	Privkey := "0xb94ae286a508e4bb3fbbcb61997822fea6f0a534510597ef8eb60a19d6b219a0"
	priv, err := cr.PrivKeyFromBytes(common.FromHex(Privkey))
	if err != nil {
		walletlog.Error("ProcImportPrivKey", "PrivKeyFromBytes err", err)
	}
	addr := account.PubKeyToAddress(priv.PubKey().Bytes())

	var acc types.Account
	acc.Addr = addr.String()
	acc.Balance = int64(10)
	acc.Currency = int32(10)
	acc.Frozen = int64(10)

	accs[total] = &acc

	// 存入账户信息到mavl树中
	hash := SaveAccountTomavl(q, nil, accs)
	walletlog.Info("TestProcCreatNewAccount", "hash", hash)

	//测试ProcGetAccountList函数
	Accounts, err := wallet.ProcGetAccountList()
	if err == nil && Accounts != nil {
		for _, Account := range Accounts.Wallets {
			walletlog.Info("TestProcCreatNewAccount:", "Account", Account.String())
		}
	}
	//测试GetAccountByLabel函数
	for i := 0; i < total; i++ {
		label := fmt.Sprintf("hybaccount:%d", i)
		Account, err := wallet.walletStore.GetAccountByLabel(label)
		if err == nil && Accounts != nil {
			walletlog.Info("TestProcCreatNewAccount:", "label", label, "label->Account", Account.String())
		}
	}
	//测试GetAccountByAddr函数
	for i := 0; i < total; i++ {
		Account, err := wallet.walletStore.GetAccountByAddr(addres[i])
		if err == nil && Account != nil {
			walletlog.Info("TestProcCreatNewAccount:", "addr", addres[i], "Addr->Account", Account.String())

		}
	}

	walletlog.Info("TestProcCreatNewAccount end --------------------")
	wallet.Close()
	stor.Close()
}

func TestProcImportPrivKey(t *testing.T) {
	walletlog.Info("TestProcImportPrivKey begin --------------------")
	wallet, q := initEnv()

	stor := storeModProc(q)
	blockchainModProc(q)

	var PrivKey types.ReqWalletImportPrivKey

	//生成一个pubkey然后换算成对应的addr
	cr, err := crypto.New(types.GetSignatureTypeName(types.SECP256K1))
	if err != nil {
		walletlog.Error("TestProcImportPrivKey", "err", err)
	}

	priv, err := cr.GenKey()
	if err != nil {
		walletlog.Error("TestProcImportPrivKey GenKey", "err", err)
	}

	PrivKey.Privkey = common.ToHex(priv.Bytes())
	PrivKey.Label = "ImportPrivKey-Label"
	walletlog.Info("TestProcImportPrivKey", "Privkey", PrivKey.Privkey, "Label", PrivKey.Label)

	time.Sleep(time.Second * 1)
	//(*types.WalletAccount, error) {
	WalletAccount, err := wallet.ProcImportPrivKey(&PrivKey)
	if err == nil {
		walletlog.Info("TestProcImportPrivKey", "WalletAccount", WalletAccount.String())
	}

	//import privkey="0xb94ae286a508e4bb3fbbcb61997822fea6f0a534510597ef8eb60a19d6b219a0"
	PrivKey.Privkey = "0xb94ae286a508e4bb3fbbcb61997822fea6f0a534510597ef8eb60a19d6b219a0"
	PrivKey.Label = "ImportPrivKey-Label-hyb"
	walletlog.Info("TestProcImportPrivKey", "Privkey", PrivKey.Privkey, "Label", PrivKey.Label)

	time.Sleep(time.Second * 1)
	//(*types.WalletAccount, error) {
	WalletAccount, err = wallet.ProcImportPrivKey(&PrivKey)
	if err == nil {
		walletlog.Info("TestProcImportPrivKey", "WalletAccount", WalletAccount.String())
	}

	time.Sleep(time.Second * 5)

	//测试ProcGetAccountList函数
	Accounts, err := wallet.ProcGetAccountList()
	if err == nil && Accounts != nil {
		for _, Account := range Accounts.Wallets {
			walletlog.Info("TestProcImportPrivKey:", "Account", Account.String())
		}
	}

	walletlog.Info("TestProcImportPrivKey end --------------------")
	wallet.Close()
	stor.Close()
}

func TestProcWalletTxList(t *testing.T) {
	walletlog.Info("TestProcWalletTxList begin --------------------")
	wallet, _ := initEnv()
	var TxList types.ReqWalletTransactionList
	TxList.Count = 5

	TxList.Direction = 1 //
	TxList.FromTx = []byte("")
	var FromTxstr string

	walletlog.Info("TestProcWalletTxList dir last-------")
	//(*types.WalletTxDetails, error)
	WalletTxDetails, err := wallet.ProcWalletTxList(&TxList)
	if err == nil {
		for _, WalletTxDetail := range WalletTxDetails.TxDetails {
			walletlog.Info("TestProcWalletTxList", "Direction", TxList.Direction, "WalletTxDetail", WalletTxDetail.String())
			FromTxstr = fmt.Sprintf("%018d", WalletTxDetail.GetHeight()*100000+WalletTxDetail.GetIndex())
		}
	}

	TxList.Direction = 1 //
	TxList.FromTx = []byte(FromTxstr)

	walletlog.Info("TestProcWalletTxList dir next-------")
	//(*types.WalletTxDetails, error)
	WalletTxDetails, err = wallet.ProcWalletTxList(&TxList)
	if err == nil {
		for _, WalletTxDetail := range WalletTxDetails.TxDetails {
			walletlog.Info("TestProcWalletTxList", "Direction", TxList.Direction, "WalletTxDetail", WalletTxDetail.String())
		}
	}
	walletlog.Info("TestProcWalletTxList dir prv------")
	//TxList.Direction = 0
	TxList.Direction = 0
	WalletTxDetails, err = wallet.ProcWalletTxList(&TxList)
	if err == nil {
		for _, WalletTxDetail := range WalletTxDetails.TxDetails {
			walletlog.Info("TestProcWalletTxList", "Direction", TxList.Direction, "WalletTxDetail", WalletTxDetail.String())
		}
	}
	walletlog.Info("TestProcWalletTxList end --------------------")
	wallet.Close()
}

//(SendToAddress *types.ReqWalletSendToAddress) (*types.ReplyHash, error) {
func TestProcSendToAddress(t *testing.T) {
	walletlog.Info("TestProcSendToAddress begin --------------------")
	wallet, q := initEnv()
	//启动mempoolModProc
	mempoolModProc(q)
	var SendToAddress types.ReqWalletSendToAddress
	SendToAddress.Amount = 1000
	SendToAddress.From = FromAddr
	SendToAddress.Note = "test"
	SendToAddress.To = "1L1zEgVcjqdM2KkQixENd7SZTaudKkcyDu"
	ReplyHash, err := wallet.ProcSendToAddress(&SendToAddress)
	if err == nil {
		walletlog.Info("TestProcSendToAddress", "ReplyHash", ReplyHash)
	}
	walletlog.Info("TestProcSendToAddress end --------------------")
	wallet.Close()
}

//ProcWalletSetFee(WalletSetFee *types.ReqWalletSetFee) error {
func TestProcWalletSetFee(t *testing.T) {
	walletlog.Info("TestProcWalletSetFee begin --------------------")
	wallet, _ := initEnv()
	var WalletSetFee types.ReqWalletSetFee
	WalletSetFee.Amount = 90000000
	err := wallet.ProcWalletSetFee(&WalletSetFee)
	if err == nil {
		walletlog.Info("TestProcWalletSetFee success")
	}
	walletlog.Info("TestProcWalletSetFee!", "FeeAmount", wallet.FeeAmount)

	walletlog.Info("TestProcWalletSetFee end --------------------")
	wallet.Close()
}

//ProcWalletSetLabel(SetLabel *types.ReqWalletSetLabel) (*types.WalletAccount, error)
func TestProcWalletSetLabel(t *testing.T) {
	walletlog.Info("TestProcWalletSetLabel begin --------------------")
	wallet, q := initEnv()

	stor := storeModProc(q)
	blockchainModProc(q)
	var SetLabel types.ReqWalletSetLabel
	SetLabel.Addr = FromAddr
	SetLabel.Label = "hybaccount:000"

	Acc, err := wallet.ProcWalletSetLabel(&SetLabel)
	if err == nil && Acc != nil {
		walletlog.Info("TestProcWalletSetLabel success", "account", Acc.String())

	} else if err != nil {
		walletlog.Info("TestProcWalletSetLabel", "err", err)
	}
	//测试ProcGetAccountList函数
	Accounts, err := wallet.ProcGetAccountList()
	if err == nil && Accounts != nil {
		for _, Account := range Accounts.Wallets {
			walletlog.Info("TestProcWalletSetLabel:", "Account", Account.String())
		}
	}

	walletlog.Info("TestProcWalletSetLabel end --------------------")
	wallet.Close()
	stor.Close()
}

//ProcMergeBalance(MergeBalance *types.ReqWalletMergeBalance) (*types.ReplyHashes, error) {
func TestProcMergeBalance(t *testing.T) {
	walletlog.Info("TestProcMergeBalance begin --------------------")
	wallet, q := initEnv()

	mempoolModProc(q)
	stor := storeModProc(q)
	blockchainModProc(q)

	var MergeBalance types.ReqWalletMergeBalance
	MergeBalance.To = "14ZTV2wHG3uPHnA5cBJmNxAxxvbzS7Z5mE"

	hashs, err := wallet.ProcMergeBalance(&MergeBalance)
	if err == nil {
		for _, hash := range hashs.Hashes {
			walletlog.Info("TestProcMergeBalance", "hash", hash)
		}
	}
	walletlog.Info("TestProcMergeBalance end --------------------")
	wallet.Close()
	stor.Close()
}

//ProcWalletSetPasswd(Passwd *types.ReqWalletSetPasswd) error {
func TestProcWalletSetPasswd(t *testing.T) {
	walletlog.Info("TestProcWalletSetPasswd begin --------------------")
	wallet, _ := initEnv()
	var Passwd types.ReqWalletSetPasswd
	Passwd.Oldpass = ""
	Passwd.Newpass = "Newpass"

	err := wallet.ProcWalletSetPasswd(&Passwd)
	if err == nil {
		walletlog.Info("TestProcWalletSetPasswd ok ", "Newpass", wallet.Password)
	}
	walletlog.Info("TestProcWalletSetPasswd end --------------------")
	wallet.Close()
}

//ProcWalletLock
func TestProcWalletLock(t *testing.T) {
	walletlog.Info("TestProcWalletLock begin --------------------")
	wallet, _ := initEnv()

	err := wallet.ProcWalletLock()
	if err == nil {
		walletlog.Info("ProcWalletLock ok ")
	}
	_, err = wallet.ProcGetAccountList()
	if err == nil {
		walletlog.Info("TestProcWalletLock", "err", err)
	}
	//解锁
	var WalletUnLock types.WalletUnLock
	WalletUnLock.Passwd = wallet.Password
	WalletUnLock.Timeout = 1
	err = wallet.ProcWalletUnLock(&WalletUnLock)
	if err == nil {
		walletlog.Info("ProcWalletUnLock ok")
	}
	flag := 0
	//测试timeout
	for {
		var WalletSetFee types.ReqWalletSetFee
		WalletSetFee.Amount = 10000000
		err := wallet.ProcWalletSetFee(&WalletSetFee)
		if err == nil {
			if flag == 0 {
				walletlog.Info("ProcWalletSetFee success")
				flag = 1
			}
		} else {
			walletlog.Info("ProcWalletSetFee", "err", err)
			break
		}
	}

	walletlog.Info("TestProcWalletLock end --------------------")
	wallet.Close()
}
