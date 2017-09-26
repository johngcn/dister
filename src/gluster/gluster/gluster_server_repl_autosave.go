// 数据同步需要注意的是：
// leader只有在通知完所有follower更新完数据之后，自身才会进行数据更新
// 因此leader
package gluster

import (
    "g/encoding/gjson"
    "time"
    "g/os/gfile"
    "g/os/glog"
    "g/encoding/gcompress"
    "sync"
    "encoding/json"
    "g/os/gcache"
)

// 日志自动保存处理
func (n *Node) autoSavingHandler() {
    go func() {
        // 初始化LastSavedId
        n.setLastSavedLogId(n.getLastLogId())
        for {
            if n.getLastSavedLogId() != n.getLastLogId() {
                n.saveLogList()
            }
            time.Sleep(gLOG_REPL_AUTOSAVE_INTERVAL * time.Millisecond)
        }
    }()

    lastLogId     := n.getLastLogId()
    lastServiceId := n.getLastServiceLogId()
    for {
        if n.getLastLogId() != lastLogId {
            n.saveDataToFile()
            lastLogId = n.getLastLogId()
        }
        if n.getLastServiceLogId() != lastServiceId {
            n.saveServiceToFile()
            lastServiceId = n.getLastServiceLogId()
        }
        time.Sleep(gLOG_REPL_AUTOSAVE_INTERVAL * time.Millisecond)
    }
}

// 定期物理化存储日志列表
func (n *Node) saveLogList() {
    // 构造数据集合
    savedid := n.getLastSavedLogId()
    lastid  := savedid
    m := make(map[int][]byte)
    p := n.LogList.Back()
    for p != nil {
        entry := p.Value.(*LogEntry)
        if entry.Id > lastid {
            s, err := json.Marshal(*entry)
            if err != nil {
                glog.Error("json marshal log entry error:", err)
                break;
            }
            s       = append(s, 10)
            n      := n.getLogEntryBatachNo(entry.Id)
            m[n]    = append(m[n], s...)
            savedid = entry.Id
        }
        p = p.Prev()
    }
    // 批量写入
    if len(m) > 0 {
        for k, v := range m {
            gfile.PutBinContentsAppend(n.getLogEntryFileSavePathByBatchNo(k), v)
        }
        n.setLastSavedLogId(savedid)
    }
}

// 保存数据到磁盘
func (n *Node) saveDataToFile() {
    key := "auto_saving_data"
    if gcache.Get(key) != nil {
        return
    }
    gcache.Set(key, struct {}{}, 6000000)
    defer gcache.Remove(key)

    data := make(map[string]interface{})
    data  = map[string]interface{} {
        "LastLogId" : n.getLastLogId(),
        "DataMap"   : *n.DataMap.Clone(),
    }
    content := []byte(gjson.Encode(&data))
    if gCOMPRESS_SAVING {
        content = gcompress.Zlib(content)
    }
    err := gfile.PutBinContents(n.getDataFilePath(), content)
    if err != nil {
        glog.Error("saving data error:", err)
    }
}

// 保存Service到磁盘
func (n *Node) saveServiceToFile() {
    key := "auto_saving_service"
    if gcache.Get(key) != nil {
        return
    }
    gcache.Set(key, struct {}{}, 6000000)
    defer gcache.Remove(key)

    data := make(map[string]interface{})
    data  = map[string]interface{} {
        "LastServiceLogId"  : n.getLastServiceLogId(),
        "Service"           : *n.Service.Clone(),
    }
    content := []byte(gjson.Encode(&data))
    if gCOMPRESS_SAVING {
        content = gcompress.Zlib(content)
    }
    err := gfile.PutBinContents(n.getServiceFilePath(), content)
    if err != nil {
        glog.Error("saving service error:", err)
    }
}

// 从物理化文件中恢复变量
func (n *Node) restoreFromFile() {
    var wg sync.WaitGroup

    wg.Add(1)
    go func() {
        n.restoreDataMap()
        wg.Done()
    }()

    wg.Add(1)
    go func() {
        n.restoreService()
        wg.Done()
    }()
    wg.Wait()
}

// 恢复DataMap
func (n *Node) restoreDataMap() {
    path := n.getDataFilePath()
    if gfile.Exists(path) {
        bin := gfile.GetBinContents(path)
        if gCOMPRESS_SAVING {
            bin = gcompress.UnZlib(bin)
        }
        if bin != nil && len(bin) > 0 {
            //glog.Println("restore data from", path)
            m := make(map[string]string)
            j := gjson.DecodeToJson(string(bin))
            n.setLastLogId(j.GetInt64("LastLogId"))
            if err := j.GetToVar("DataMap", &m); err == nil {
                n.DataMap.BatchSet(m)
            } else {
                glog.Error(err)
            }
        }
    }
}

// 恢复Service
func (n *Node) restoreService() {
    path := n.getServiceFilePath()
    if gfile.Exists(path) {
        bin := gfile.GetBinContents(path)
        if gCOMPRESS_SAVING {
            bin = gcompress.UnZlib(bin)
        }
        if bin != nil && len(bin) > 0 {
            //glog.Println("restore service from", path)
            m := make(map[string]Service)
            j := gjson.DecodeToJson(string(bin))
            n.setLastServiceLogId(j.GetInt64("LastServiceLogId"))
            if err := j.GetToVar("Service", &m); err == nil {
                for k, v := range m {
                    n.Service.Set(k, v)
                }
            } else {
                glog.Error(err)
            }
        }
    }
}


