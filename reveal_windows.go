// 탐색기에서 파일 고르기('폴더 열기', UI 설계 5.5절). Windows 전용이라 파일을 나눈다.
package main

import (
	"cmp"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// reveal은 explorer.exe /select,"<경로>"로 탐색기를 띄워 path를 고른다. Go가 인자를 통째로 따옴표로 싸면 /select,가
// 먹지 않아 명령 줄을 직접 쓴다. path는 방금 파일을 쓴 경로(RevealFile)라 Windows가 받는 이름이고, 그래서 "가 들어갈 수 없다.
// explorer는 성공해도 종료 코드 1을 돌려줘 기다리지 않는다.
// SHOpenFolderAndSelectItems는 부르는 스레드에서 COM을 초기화해야 해서 바인딩 고루틴에서 쓰기 번거롭다(UI 계획 P9).
func reveal(path string) error {
	exe := filepath.Join(cmp.Or(os.Getenv("SystemRoot"), `C:\Windows`), "explorer.exe")
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + exe + `" /select,"` + path + `"`}
	if err := cmd.Start(); err != nil {
		return errReveal
	}
	go cmd.Wait() // 끝난 프로세스를 거둔다
	return nil
}
