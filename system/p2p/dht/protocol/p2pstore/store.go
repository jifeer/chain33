package p2pstore

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	types2 "github.com/33cn/chain33/system/p2p/dht/types"
	"github.com/33cn/chain33/types"
	"github.com/ipfs/go-datastore"
	kb "github.com/libp2p/go-libp2p-kbucket"
)

const (
	LocalChunkInfoKey = "local-chunk-info"
	ChunkNameSpace    = "chunk"
	AlphaValue        = 3
	Backup            = 3
)

// 保存chunk到本地p2pStore，同时更新本地chunk列表
func (p *Protocol) addChunkBlock(info *types.ChunkInfoMsg, bodys *types.BlockBodys) error {
	//先检查数据是不是正在保存
	if _, ok := p.saving.LoadOrStore(hex.EncodeToString(info.ChunkHash), nil); ok {
		return nil
	}
	defer p.saving.Delete(hex.EncodeToString(info.ChunkHash))
	b := types.Encode(&types.P2PStoreData{
		Time: time.Now().UnixNano(),
		Data: &types.P2PStoreData_BlockBodys{BlockBodys: bodys},
	})

	err := p.addLocalChunkInfo(info)
	if err != nil {
		return err
	}
	return p.DB.Put(genChunkKey(info.ChunkHash), b)
}

// 更新本地chunk保存时间，chunk不存在则返回error
func (p *Protocol) updateChunk(req *types.ChunkInfoMsg) error {
	b, err := p.DB.Get(genChunkKey(req.ChunkHash))
	if err != nil {
		return err
	}
	var data types.P2PStoreData
	err = types.Decode(b, &data)
	if err != nil {
		return err
	}
	return p.addChunkBlock(req, data.Data.(*types.P2PStoreData_BlockBodys).BlockBodys)
}

// 获取本地chunk数据，若数据已过期则删除该数据并返回空
func (p *Protocol) getChunkBlock(hash []byte) (*types.BlockBodys, error) {
	b, err := p.DB.Get(genChunkKey(hash))
	if err != nil {
		return nil, err
	}
	var data types.P2PStoreData
	err = types.Decode(b, &data)
	if err != nil {
		return nil, err
	}
	if time.Now().UnixNano()-data.Time > int64(types2.ExpiredTime) {
		err = p.deleteChunkBlock(hash)
		if err != nil {
			log.Error("getChunkBlock", "delete chunk error", err, "hash", hex.EncodeToString(hash))
		}
		err = types2.ErrExpired
	}

	return data.Data.(*types.P2PStoreData_BlockBodys).BlockBodys, err

}

func (p *Protocol) deleteChunkBlock(hash []byte) error {
	err := p.deleteLocalChunkInfo(hash)
	if err != nil {
		return err
	}
	return p.DB.Delete(genChunkKey(hash))
}

// 保存一个本地chunk hash列表，用于遍历本地数据
func (p *Protocol) addLocalChunkInfo(info *types.ChunkInfoMsg) error {
	hashMap, err := p.getLocalChunkInfoMap()
	if err != nil {
		return err
	}

	if _, ok := hashMap[hex.EncodeToString(info.ChunkHash)]; ok {
		return nil
	}

	hashMap[hex.EncodeToString(info.ChunkHash)] = info
	return p.saveLocalChunkInfoMap(hashMap)
}

func (p *Protocol) deleteLocalChunkInfo(hash []byte) error {
	hashMap, err := p.getLocalChunkInfoMap()
	if err != nil {
		return err
	}
	delete(hashMap, hex.EncodeToString(hash))
	return p.saveLocalChunkInfoMap(hashMap)
}

func (p *Protocol) getLocalChunkInfoMap() (map[string]*types.ChunkInfoMsg, error) {

	value, err := p.DB.Get(datastore.NewKey(LocalChunkInfoKey))
	if err != nil {
		return make(map[string]*types.ChunkInfoMsg), nil
	}

	var chunkInfoMap map[string]*types.ChunkInfoMsg
	err = json.Unmarshal(value, &chunkInfoMap)
	if err != nil {
		return nil, err
	}

	return chunkInfoMap, nil
}

func (p *Protocol) saveLocalChunkInfoMap(m map[string]*types.ChunkInfoMsg) error {
	value, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return p.DB.Put(datastore.NewKey(LocalChunkInfoKey), value)
}

// 适配libp2p，按路径格式生成数据的key值，便于区分多种数据类型的命名空间，以及key值合法性校验
func genChunkPath(hash []byte) string {
	return fmt.Sprintf("/%s/%s", ChunkNameSpace, hex.EncodeToString(hash))
}

func genChunkKey(hash []byte) datastore.Key {
	return datastore.NewKey(genChunkPath(hash))
}

func genDHTID(chunkHash []byte) kb.ID {
	return kb.ConvertKey(genChunkPath(chunkHash))
}
