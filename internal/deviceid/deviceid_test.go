package deviceid

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestLoadGeneratesAndPersists(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "device.key")

	d1, err := Load(&FileStore{Path: path})
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	d2, err := Load(&FileStore{Path: path})
	if err != nil {
		t.Fatalf("第二次 Load() 出错: %v", err)
	}
	if d1.ID() != d2.ID() {
		t.Errorf("两次加载的设备 ID 不一致: %s != %s", d1.ID(), d2.ID())
	}
	if !bytes.Equal(d1.PublicKey(), d2.PublicKey()) {
		t.Error("两次加载的公钥不一致")
	}
}

func TestIDFormat(t *testing.T) {
	keyring.MockInit()
	d, err := Load(&FileStore{Path: filepath.Join(t.TempDir(), "device.key")})
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	if got := len(d.ID()); got != 32 {
		t.Errorf("ID 长度 = %d, 期望 32", got)
	}
	for _, c := range d.ID() {
		// base32 小写字母 a-z 和数字 2-7。
		if !(c >= 'a' && c <= 'z') && !(c >= '2' && c <= '7') {
			t.Errorf("ID 含非法字符 %q: %s", c, d.ID())
			break
		}
	}
}

func TestIDKnownValue(t *testing.T) {
	// 用 RFC 8032 ed25519 测试向量 1 的种子构造私钥，验证 ID 的计算方式。
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	priv := ed25519.NewKeyFromSeed(seed)
	d := &Device{priv: priv}
	// 期望值：SHA-256(公钥)[:20] 的 base32 小写无填充。
	want := "eh7ddx5bksrgcytl7bkai36se4nxx3kl"
	if d.ID() != want {
		t.Errorf("ID() = %s, 期望 %s", d.ID(), want)
	}
}

func TestSign(t *testing.T) {
	keyring.MockInit()
	d, err := Load(&FileStore{Path: filepath.Join(t.TempDir(), "device.key")})
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	msg := []byte("hello")
	if !ed25519.Verify(d.PublicKey(), msg, d.Sign(msg)) {
		t.Error("签名验证失败")
	}
	if ed25519.Verify(d.PublicKey(), []byte("other"), d.Sign(msg)) {
		t.Error("错误消息不应通过验证")
	}
}

func TestFileStorePermissions(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "device.key")
	if _, err := Load(&FileStore{Path: path}); err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() 出错: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("文件权限 = %o, 期望 600", info.Mode().Perm())
	}
}

func TestKeyringStore(t *testing.T) {
	keyring.MockInit()
	st := KeyringStore{}

	if _, err := st.Get(); !errors.Is(err, ErrNotFound) {
		t.Errorf("未保存时 Get() = %v, 期望 ErrNotFound", err)
	}
	if err := st.Set([]byte("test-data")); err != nil {
		t.Fatalf("Set() 出错: %v", err)
	}
	got, err := st.Get()
	if err != nil {
		t.Fatalf("Get() 出错: %v", err)
	}
	if string(got) != "test-data" {
		t.Errorf("Get() = %q, 期望 test-data", got)
	}
}

func TestLoadFromKeyring(t *testing.T) {
	keyring.MockInit()

	d1, err := Load(KeyringStore{})
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	d2, err := Load(KeyringStore{})
	if err != nil {
		t.Fatalf("第二次 Load() 出错: %v", err)
	}
	if d1.ID() != d2.ID() {
		t.Errorf("密钥库中的设备 ID 不一致: %s != %s", d1.ID(), d2.ID())
	}
}

func TestDefaultStoreFallsBackToFile(t *testing.T) {
	// 模拟系统密钥库不可用（如 Linux 服务器没有 Secret Service）。
	keyring.MockInitWithError(errors.New("dbus 不可用"))
	path := filepath.Join(t.TempDir(), "device.key")

	d1, err := Load(DefaultStore(path))
	if err != nil {
		t.Fatalf("密钥库不可用时 Load() 出错: %v", err)
	}
	// 私钥应已退回文件保存。
	if _, err := os.Stat(path); err != nil {
		t.Errorf("退回文件未创建: %v", err)
	}
	d2, err := Load(DefaultStore(path))
	if err != nil {
		t.Fatalf("第二次 Load() 出错: %v", err)
	}
	if d1.ID() != d2.ID() {
		t.Errorf("退回文件后设备 ID 不一致: %s != %s", d1.ID(), d2.ID())
	}
}

func TestDefaultStorePrefersKeyring(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "device.key")

	d1, err := Load(DefaultStore(path))
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	// 密钥库可用时不应写文件。
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("密钥库可用时不应创建文件: %v", err)
	}
	d2, err := Load(DefaultStore(path))
	if err != nil {
		t.Fatalf("第二次 Load() 出错: %v", err)
	}
	if d1.ID() != d2.ID() {
		t.Errorf("设备 ID 不一致: %s != %s", d1.ID(), d2.ID())
	}
}

func TestDefaultStoreReadsFileWhenKeyringEmpty(t *testing.T) {
	// 密钥库可用但为空，文件里有早先保存的私钥：应读文件而不是重新生成。
	keyring.MockInit()
	dir := t.TempDir()
	path := filepath.Join(dir, "device.key")

	want, err := Load(&FileStore{Path: path})
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	got, err := Load(DefaultStore(path))
	if err != nil {
		t.Fatalf("Load() 出错: %v", err)
	}
	if want.ID() != got.ID() {
		t.Errorf("文件中的设备 ID 未被读取: %s != %s", want.ID(), got.ID())
	}
}
