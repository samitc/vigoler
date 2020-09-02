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

func runCommand(ctx context.Context, command string, createChan, readWithDelim, closeReader, callWait bool, arg ...string) (WaitAble, io.ReadCloser, <-chan string, error) {
	cmd := exec.CommandContext(ctx, command, arg...)
	var outputChannel chan string
	var reader io.ReadCloser
	var writer io.WriteCloser
	var err error
	if createChan || !closeReader {
		reader, writer, err = os.Pipe()
		if err != nil {
			return nil, nil, nil, err
		}
		defer writer.Close()
		cmd.Stdout = writer
		cmd.Stderr = writer
		if createChan {
			outputChannel = make(chan string)
			go func(outChan chan string, reader io.ReadCloser, readWithDelim bool) {
				if closeReader {
					defer reader.Close()
				}
				var err error
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
			}(outputChannel, reader, readWithDelim)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	if callWait {
		go func(cmd *exec.Cmd, app string, args ...string) {
			err := cmd.Wait()
			if err != nil {
				fmt.Printf("Error in app %s app with args: %s and errro: %v.\n", app, strings.Join(args, ","), err)
			}
		}(cmd, command, arg...)
	}
	return &commandWaitAble{cmd: cmd}, reader, outputChannel, nil
}

// TODO: remove option to get reader stream
func (external *externalApp) runCommand(ctx context.Context, createChan bool, closeReader bool, readWithDelim bool, arg ...string) (WaitAble, io.ReadCloser, <-chan string, error) {
	waitAble, _, oChan, err := runCommand(ctx, external.appLocation, createChan, readWithDelim, closeReader, true, arg...)
	return waitAble, nil, oChan, err
}
func (external *externalApp) runCommandWait(ctx context.Context, arg ...string) (WaitAble, error) {
	wait, _, _, err := runCommand(ctx, external.appLocation, false, true, true, false, arg...)
	return wait, err
}
func (external *externalApp) runCommandChan(ctx context.Context, arg ...string) (WaitAble, <-chan string, error) {
	waitAble, _, channel, err := external.runCommand(ctx, true, true, true, arg...)
	return waitAble, channel, err
}
func (external *externalApp) runCommandReadWait(ctx context.Context, arg ...string) (WaitAble, io.ReadCloser, error) {
	wait, reader, _, err := runCommand(ctx, external.appLocation, false, false, false, false, arg...)
	return wait, reader, err
}
func (external *externalApp) runCommandRead(ctx context.Context, wait bool, args ...string) (WaitAble, <-chan string, error) {
	waitAble, _, channel, err := runCommand(ctx, external.appLocation, true, false, true, wait, args...)
	return waitAble, channel, err
}
