package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/zalando/go-keyring"
)

const credentialFile = "secrets.json"

type fileBackend struct {
	dir string
	mu  sync.Mutex
}

func newFileBackend(dir string) Backend { return &fileBackend{dir: dir} }

func (*fileBackend) Storage() StorageKind { return StorageFile }

func (b *fileBackend) Get(user string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	root, err := b.open(false)
	if err != nil {
		return "", err
	}
	defer root.Close()
	values, err := readCredentials(root)
	if err != nil {
		return "", err
	}
	value, ok := values[user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (b *fileBackend) Set(user, password string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	root, err := b.open(true)
	if err != nil {
		return err
	}
	defer root.Close()
	values, err := readCredentials(root)
	if errors.Is(err, keyring.ErrNotFound) {
		values = make(map[string]string)
	} else if err != nil {
		return err
	}
	values[user] = password
	return writeCredentials(root, values)
}

func (b *fileBackend) Delete(user string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	root, err := b.open(false)
	if err != nil {
		return err
	}
	defer root.Close()
	values, err := readCredentials(root)
	if err != nil {
		return err
	}
	if _, ok := values[user]; !ok {
		return keyring.ErrNotFound
	}
	delete(values, user)
	// 保留空对象，作为后续启动继续使用文件后端的标记。
	return writeCredentials(root, values)
}

func (b *fileBackend) open(create bool) (*os.Root, error) {
	info, err := os.Lstat(b.dir)
	if create && errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(b.dir, 0700); err != nil {
			return nil, err
		}
		info, err = os.Lstat(b.dir)
	}
	if err != nil {
		return nil, credentialMissing(err)
	}
	if !info.IsDir() {
		return nil, errors.New("credential storage directory is not a regular directory")
	}
	base, err := os.OpenRoot(b.dir)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	if err = checkCredentialDirectory(base, info, false); err != nil {
		return nil, err
	}
	info, err = base.Lstat("credentials")
	if create && errors.Is(err, os.ErrNotExist) {
		if err = base.Mkdir("credentials", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, err = base.Lstat("credentials")
	}
	if err != nil {
		return nil, credentialMissing(err)
	}
	if !info.IsDir() {
		return nil, errors.New("credential storage directory is not a regular directory")
	}
	root, err := base.OpenRoot("credentials")
	if err != nil {
		return nil, err
	}
	if err = checkCredentialDirectory(root, info, true); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

func checkCredentialDirectory(root *os.Root, expected os.FileInfo, secure bool) error {
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil {
		return err
	}
	if !actual.IsDir() || !os.SameFile(expected, actual) {
		return errors.New("credential storage directory changed during access")
	}
	if secure {
		return secureCredentialFile(f, true)
	}
	return nil
}

func credentialMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return keyring.ErrNotFound
	}
	return err
}

func readCredentials(root *os.Root) (map[string]string, error) {
	info, err := root.Lstat(credentialFile)
	if err != nil {
		return nil, credentialMissing(err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("credential storage is not a regular file")
	}
	f, err := root.Open(credentialFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, errors.New("credential storage changed during access")
	}
	if err = secureCredentialFile(f, false); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(f)
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalidCredentialData()
	}
	values := make(map[string]string)
	for decoder.More() {
		key, keyErr := decoder.Token()
		value, valueErr := decoder.Token()
		user, keyOK := key.(string)
		password, valueOK := value.(string)
		if keyErr != nil || valueErr != nil || !keyOK || !valueOK {
			return nil, invalidCredentialData()
		}
		if _, duplicate := values[user]; duplicate {
			return nil, invalidCredentialData()
		}
		values[user] = password
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, invalidCredentialData()
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, invalidCredentialData()
	}
	return values, nil
}

func invalidCredentialData() error {
	// 解码错误可能包含输入片段，不能把凭据内容带入错误信息。
	return ErrCredentialCorrupt
}

func writeCredentials(root *os.Root, values map[string]string) error {
	data, err := json.Marshal(values)
	if err != nil {
		return errors.New("could not encode credential storage")
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	name := ".secrets-" + hex.EncodeToString(random[:])
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	// 写入任何凭据内容前先限制权限，Windows 上也必须先设置 ACL。
	if err = secureCredentialFile(f, false); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if info, statErr := root.Lstat(credentialFile); statErr == nil {
		if !info.Mode().IsRegular() {
			return errors.New("credential storage is not a regular file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	return root.Rename(name, credentialFile)
}
