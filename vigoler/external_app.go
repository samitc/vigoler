package vigoler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type externalApp struct {
	appLocation string
}
type commandWaitAble struct {
	cmd *exec.Cmd
}

func (cwa *commandWaitAble) Wait() error {
	return cwa.cmd.Wait()
}
func (cwa *commandWaitAble) Stop() error {
	return cwa.cmd.Process.Kill()
}

// TODO: remove option to get reader stream
func (external *externalApp) runCommand(ctx context.Context, createChan bool, closeReader bool, readWithDelim bool, arg ...string) (WaitAble, io.ReadCloser, <-chan string, error) {
	cmd := exec.CommandContext(ctx, external.appLocation, arg...)
	var outputChannel chan string = nil
	var streamReader io.ReadCloser = nil
	if createChan || !closeReader {
		reader, writer, err := os.Pipe()
		if err != nil {
			return nil, nil, nil, err
		}
		streamReader = reader
		defer writer.Close()
		cmd.Stdout = writer
		cmd.Stderr = writer
		if createChan {
			outputChannel = make(chan string)
			go func(outChan chan string, reader io.ReadCloser, closeReader bool, readWithDelim bool) {
				if closeReader {
					defer reader.Close()
				}
				var err error = nil
				if readWithDelim {
					rd := bufio.NewReader(reader)
					var line string
					line, err = rd.ReadString('\n')
					for ; err == nil; line, err = rd.ReadString('\n') {
						outChan <- line
					}
					if err != io.EOF {
						panic(err)
					}
				} else {
					buf := make([]byte, 128)
					var n int
					for err == nil {
						n, err = reader.Read(buf)
						if err == nil {
							outChan <- string(buf[:n])
						}
					}
					if err != io.EOF {
						panic(err)
					}
				}
				close(outChan)
			}(outputChannel, reader, closeReader, readWithDelim)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	go func(cmd *exec.Cmd, app string, args ...string) {
		err := cmd.Wait()
		if err != nil {
			fmt.Println("error in app:" + app + " with args:" + strings.Join(args, ","))
			fmt.Println(err)
		}
	}(cmd, external.appLocation, arg...)
	return &commandWaitAble{cmd: cmd}, streamReader, outputChannel, nil
}
func (external *externalApp) runCommandWait(ctx context.Context, arg ...string) (WaitAble, error) {
	wait, _, _, err := external.runCommand(ctx, false, true, true, arg...)
	return wait, err
}
func (external *externalApp) runCommandChan(ctx context.Context, arg ...string) (WaitAble, <-chan string, error) {
	waitAble, _, channel, err := external.runCommand(ctx, true, true, true, arg...)
	return waitAble, channel, err
}
