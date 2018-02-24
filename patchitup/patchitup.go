package patchitup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	humanize "github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/schollz/utils"
)

type Patchitup struct {
	username      string
	serverAddress string
	filename      string
	cacheFolder   string
	hashLocal     string
}

func New(address, username string, server ...bool) (p *Patchitup) {
	p = new(Patchitup)
	p.cacheFolder = path.Join(utils.UserHomeDir(), ".patchitup", "client", username)
	if len(server) > 0 && server[0] {
		p.cacheFolder = path.Join(utils.UserHomeDir(), ".patchitup", "server", username)
	}
	os.MkdirAll(p.cacheFolder, 0755)
	p.username = username
	p.serverAddress = address
	return p
}

type patchFile struct {
	Filename  string
	Hash      string
	Timestamp int
}

func (p *Patchitup) Rebuild(pathToFile string) (err error) {
	// flush logs so that they show up
	defer log.Flush()

	log.Debug("rebuilding")

	// generate the filename
	_, p.filename = filepath.Split(pathToFile)

	patches, err := p.getPatches()
	if err != nil {
		return
	}
	log.Debugf("patches: %+v", patches)

	currentText := ""
	for _, patch := range patches {
		var patchBytes []byte
		var patchString string
		log.Debugf("adding %s from %d", patch.Hash, patch.Timestamp)
		patchBytes, err = ioutil.ReadFile(patch.Filename)
		if err != nil {
			return
		}
		patchString, err = decode(string(patchBytes))
		if err != nil {
			return
		}
		currentText, err = patchText(currentText, patchString)
		if err != nil {
			return
		}
		log.Debug([]byte(currentText))
		log.Debugf("rebuilt hash: %s", utils.Md5Sum(currentText))
		log.Debugf("supposed hash: %s", patch.Hash)
	}
	b, _ := ioutil.ReadFile("test.txt")
	log.Debug(b)
	return
}

// getPatches will determine the hash of the latest file
func (p *Patchitup) getPatches() (patchFiles []patchFile, err error) {
	files, err := ioutil.ReadDir(p.cacheFolder)
	if err != nil {
		return
	}

	m := make(map[int]patchFile)
	for _, f := range files {
		if !strings.HasPrefix(f.Name(), p.filename+".") {
			continue
		}
		g := strings.Split(f.Name(), ".")
		if len(g) < 4 {
			continue
		}
		var err2 error
		pf := patchFile{Filename: path.Join(p.cacheFolder, f.Name())}
		pf.Timestamp, err2 = strconv.Atoi(g[len(g)-1])
		if err2 != nil {
			continue
		}
		pf.Hash = g[len(g)-2]
		m[pf.Timestamp] = pf
	}

	keys := make([]int, len(m))
	i := 0
	for key := range m {
		keys[i] = key
		i++
	}
	sort.Ints(keys)

	patchFiles = make([]patchFile, len(keys))
	for i, key := range keys {
		patchFiles[i] = m[key]
	}
	return
}

// PatchUp will take a filename and upload it to the server via a patch using the specified user.
func (p *Patchitup) PatchUp(pathToFile string) (err error) {
	// flush logs so that they show up
	defer log.Flush()

	// generate the filename
	_, p.filename = filepath.Split(pathToFile)

	// first make sure the file to upload exists
	if !utils.Exists(pathToFile) {
		return fmt.Errorf("'%s' not found", pathToFile)
	}

	// copy current state of file
	err = utils.CopyFile(pathToFile, pathToFile+".temp")
	if err != nil {
		return
	}
	pathToFile = pathToFile + ".temp"

	// get current text
	localText, err := getFileText(pathToFile)
	if err != nil {
		return
	}
	localText = string(utils.Dos2Unix([]byte(localText)))

	// get current hash
	p.hashLocal = utils.Md5Sum(localText)
	if err != nil {
		return
	}

	// get current hash of the remote file and compare
	// TODO: return if they are the same

	// get info from the last version uploaded
	pathToLocalCopyOfRemote := path.Join(p.cacheFolder, p.filename+".last")
	localCopyOfRemoteText := ""
	hashLocalCopyOfRemote := ""
	if utils.Exists(pathToLocalCopyOfRemote) {
		hashLocalCopyOfRemote, err = utils.Filemd5Sum(pathToLocalCopyOfRemote)
		if err != nil {
			return
		}
		localCopyOfRemoteText, err = getFileText(pathToLocalCopyOfRemote)
		if err != nil {
			return
		}
	}

	// make sure that the hash of the local copy of the remote is the same as the one on the server
	log.Debugf("hashLocalCopyOfRemote: %s", hashLocalCopyOfRemote)
	// TODO: pull from the server if they differ

	// upload patches
	patch := getPatch(localCopyOfRemoteText, localText)
	err = p.uploadPatches(encode(patch))
	if err != nil {
		return err
	} else {
		log.Infof("patched %s (%2.1f%%) to remote '%s' for '%s'",
			humanize.Bytes(uint64(len(patch))),
			100*float64(len(patch))/float64(len(localText)),
			p.filename,
			p.username)
	}

	// update the local remote copy
	err = utils.CopyFile(pathToFile, path.Join(p.cacheFolder, p.filename+".last"))

	log.Info("remote server is up-to-date")
	return
}

// postToServer is generic function to post to the server
func postToServer(address string, sr serverRequest) (target serverResponse, err error) {
	payloadBytes, err := json.Marshal(sr)
	if err != nil {
		return
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", address, body)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&target)
	if err != nil {
		return
	}
	if !target.Success {
		err = errors.New(target.Message)
	}
	log.Debugf("POST %s: %s", address, target.Message)
	return
}

// getLatestHash will get latest hash from server
func (p *Patchitup) getLatestHash() (hashRemote string, err error) {
	sr := serverRequest{
		Username: p.username,
		Filename: p.filename,
	}
	target, err := postToServer(p.serverAddress+"/hash", sr)
	hashRemote = target.Message
	return
}

// uploadPatches will upload the patch to the server
func (p *Patchitup) uploadPatches(patch string) (err error) {
	filename := fmt.Sprintf("%s.%s.%d",
		p.filename,
		p.hashLocal,
		time.Now().UTC().UnixNano()/int64(time.Millisecond),
	)
	sr := serverRequest{
		Username: p.username,
		Filename: filename,
		Patch:    patch,
	}
	_, err = postToServer(p.serverAddress+"/patch", sr)
	return
}