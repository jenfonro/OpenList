package local

// TestDirCalculateSize tests the directory size calculation
// It should be run with the local driver enabled and directory size calculation set to true
import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
)

func generatedTestDir(dir string, dep, filecount int) {
	if dep == 0 {
		return
	}
	for i := 0; i < dep; i++ {
		subDir := dir + "/dir" + strconv.Itoa(i)
		os.Mkdir(subDir, 0755)
		generatedTestDir(subDir, dep-1, filecount)
		generatedFiles(subDir, filecount)
	}
}

func generatedFiles(path string, count int) error {
	for i := 0; i < count; i++ {
		filePath := filepath.Join(path, "file"+strconv.Itoa(i)+".txt")
		file, err := os.Create(filePath)
		if err != nil {
			return err
		}
		// 使用随机ascii字符填充文件
		content := make([]byte, 1024) // 1KB file
		for j := range content {
			content[j] = byte('a' + j%26) // Fill with 'a' to 'z'
		}
		_, err = file.Write(content)
		if err != nil {
			return err
		}
		file.Close()
	}
	return nil
}

// performance tests for directory size calculation
func BenchmarkCalculateDirSize(t *testing.B) {
	// 初始化t的日�?
	t.Logf("Starting performance test for directory size calculation")
	// 确保测试目录存在
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}
	// 创建tmp directory for testing
	testTempDir := t.TempDir()
	err := os.MkdirAll(testTempDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testTempDir) // Clean up after test
	// 构建一个深度为5，每�?0个文件和10个目录的目录结构
	generatedTestDir(testTempDir, 5, 10)
	// Initialize the local driver with directory size calculation enabled
	d := &Local{
		directoryMap: DirectoryMap{
			root: testTempDir,
		},
		Addition: Addition{
			DirectorySize: true,
			RootPath: driver.RootPath{
				RootFolderPath: testTempDir,
			},
		},
	}
	//record the start time
	t.StartTimer()
	// Calculate the directory size
	err = d.directoryMap.RecalculateDirSize()
	if err != nil {
		t.Fatalf("Failed to calculate directory size: %v", err)
	}
	//record the end time
	t.StopTimer()
	// Print the size and duration
	node, ok := d.directoryMap.Get(d.directoryMap.root)
	if !ok {
		t.Fatalf("Failed to get root node from directory map")
	}
	t.Logf("Directory size: %d bytes", node.fileSum+node.directorySum)
	t.Logf("Performance test completed successfully")
}
