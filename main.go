package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	success         = "SUCCESS"
	fail            = "FAIL"
	searchString    = "Changeset changelog/ddl.xml"
	dbTypeMysql     = "mysql"
	dbTypeSqlServer = "sqlserver"
	dbTypeOracle    = "oracle"
)

func main() {
	// 设置 JAVA_TOOL_OPTIONS 环境变量
	err := os.Setenv("JAVA_TOOL_OPTIONS", "-Dfile.encoding=UTF-8")
	if err != nil {
		fmt.Println("Error setting JAVA_TOOL_OPTIONS environment variable:", err)
		return
	}

	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current directory:", err)
		return
	}

	// 构造 liquibase 目录路径
	liquibaseDir := currentDir + "/liquibase"

	// ch 用于第一步和第二部之间两个 goroutine 通信
	ch := make(chan string)

	// 生成 changelog-ddl.xml 文件
	go generateChangeLog(liquibaseDir, ch)

	// 生成 sql
	select {
	case s := <-ch:
		if success == s {
			// 生成 changelog-ddl.xml 文件成功
			generateSql(liquibaseDir)
		} else {
			// 生成 changelog-ddl.xml 文件失败，不做处理
		}
	}

	// 执行成功，清理文件
	err = os.Remove(currentDir + "/changelog/ddl.xml")
	if err != nil {
		slog.Error("Error removing changelog:", "msg", err)
	}
}

func generateChangeLog(liquibaseDir string, ch chan string) {
	slog.Info("Start to generate changelog/ddl.xml")

	// 生成 changelog-ddl.xml 文件
	cmd := exec.Command(liquibaseDir+"/liquibase", "--changeLogFile=changelog/ddl.xml", "--defaultsFile=config/diff.properties", "diffChangeLog")
	//cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr

	out, err := cmd.CombinedOutput()
	if err != nil {
		// 执行出错向 channel 里发送通知
		ch <- fail
		slog.Error("Error executing Liquibase command diffChangeLog:", "msg", out)
		return
	}

	slog.Info("Generate changelog/ddl.xml successfully")

	// 执行完毕向 channel 里发送通知
	ch <- success
}

func generateSql(liquibaseDir string) {
	var wg sync.WaitGroup
	wg.Add(3)

	// mysql
	go func() {
		defer wg.Done()
		doGenerateSql(liquibaseDir, dbTypeMysql)
	}()

	// sqlserver
	go func() {
		defer wg.Done()
		doGenerateSql(liquibaseDir, dbTypeSqlServer)
	}()

	// oracle
	go func() {
		defer wg.Done()
		doGenerateSql(liquibaseDir, dbTypeOracle)
	}()

	wg.Wait()
}

func doGenerateSql(liquibaseDir string, dbType string) {
	slog.Info("Start to generate sql", "db", dbType)
	// 生成 changelog-ddl.xml 文件
	cmd := exec.Command(liquibaseDir+"/liquibase", "--changeLogFile=changelog/ddl.xml", fmt.Sprintf("--defaultsFile=config/%s.properties", dbType), "updateSql")

	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Error executing Liquibase command updateSql:", "msg", out)
		return
	}

	// 从 out 中解析出想要的 sql
	lines := extractNextLines(out, searchString, dbType)

	// 将 sql 写入文件
	err = writeToFile(lines, fmt.Sprintf("./out/%s.sql", dbType))
	if err != nil {
		slog.Error("Error writing to file:", "msg", err)
	}
	slog.Info("Generate sql successfully", "db", dbType)
}

func extractNextLines(data []byte, searchString string, dbType string) []string {
	var result []string

	// 提取文件内容的标志
	flag := false

	// 逐行读取文件内容，提取需要的
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if flag {
			if strings.Contains(line, "INSERT INTO") {
				flag = false
			} else {
				// 类型映射
				var sql string
				// 类型转换
				switch dbType {
				case dbTypeMysql:
					sql = convertMysql(line)
				case dbTypeSqlServer:
					sql = convertSqlServer(line)
				case dbTypeOracle:
					sql = convertOracle(line)
				}
				// 开始提取内容
				result = append(result, sql)
			}
			continue
		}
		if strings.Contains(line, searchString) {
			result = append(result, line)
			flag = true
		}
	}
	return result
}

// convertMysql 转换 mysql 的数据类型
// 自动生成的数据类型有时候不满足现状
func convertMysql(sql string) string {
	// 去除表名
	sql = removeTableName(sql, dbTypeMysql)
	return sql
}

// convertOracle 转换 oracle 的数据类型
// 自动生成的数据类型有时候不满足现状
func convertOracle(sql string) string {
	// VARCHAR2(xx) -> VARCHAR2(xx char)
	varchar2Reg := regexp.MustCompile(`VARCHAR2\([0-9]+\)`)
	varchar2Arr := varchar2Reg.FindStringSubmatch(sql)
	for _, varchar2 := range varchar2Arr {
		newStr := varchar2[0:len(varchar2)-1] + " char)"
		sql = strings.ReplaceAll(sql, varchar2, newStr)
	}
	// DECIMAL -> NUMBER
	sql = strings.ReplaceAll(sql, "DECIMAL", "NUMBER")

	// 去除表名
	sql = removeTableName(sql, dbTypeOracle)

	return sql
}

// convertSqlServer 转换 sqlServer 的数据类型
// 自动生成的数据类型有时候不满足现状
func convertSqlServer(sql string) string {
	// varchar -> nvarchar
	sql = strings.ReplaceAll(sql, "varchar", "nvarchar")
	sql = strings.ReplaceAll(sql, "nnvarchar", "nvarchar")
	// varchar (max) -> ntext
	sql = strings.ReplaceAll(sql, "varchar (max)", "ntext")
	sql = strings.ReplaceAll(sql, "varchar(MAX)", "ntext")
	sql = strings.ReplaceAll(sql, "nntext", "ntext")
	// datetime -> datetime2
	sql = strings.ReplaceAll(sql, "datetime", "datetime2")

	return sql
}

// removeTableName 去除表名
func removeTableName(sql string, dbType string) string {
	f, err := os.Open(fmt.Sprintf("config/%s.properties", dbType))
	if err != nil {
		slog.Error("Error opening file:", "msg", err)
	}
	defer f.Close()

	// 逐行读取文件内容，提取需要的
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "db") {
			arr := strings.Split(line, ": ")
			if len(arr) == 2 {
				sql = strings.ReplaceAll(sql, arr[1]+".", "")
			}
		}
	}
	return sql
}

func writeToFile(lines []string, filename string) error {
	// 将字符串数组连接成一个长字符串
	content := strings.Join(lines, "\n")

	// 创建目录
	err := os.MkdirAll(filepath.Dir(filename), 0755)
	if err != nil {
		return fmt.Errorf("error creating directories: %w", err)
	}

	// 将内容写入文件
	err = os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return err
	}

	return nil
}
