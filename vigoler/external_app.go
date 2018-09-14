package vigoler

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
)

type externalApp struct {
	appLocation string
}

func (external *externalApp) runCommand(ctx context.Context, createChan bool, arg ...string) (WaitAble, <-chan string, error) {
	cmd := exec.CommandContext(ctx, external.appLocation, arg...)
	var outputChannel chan string = nil
	if createChan {
		reader, writer, err := os.Pipe()
		if err != nil {
			return nil, nil, err
		}
		defer writer.Close()
		cmd.Stdout = writer
		cmd.Stderr = writer
		outputChannel = make(chan string)
		go func(outChan chan string, reader io.ReadCloser) {
			defer reader.Close()
			rd := bufio.NewReader(reader)
			line, err := rd.ReadString('\n')
			for ; err == nil; line, err = rd.ReadString('\n') {
				outChan <- line
			}
			if err != io.EOF {
				panic(err)
			}
			close(outChan)
		}(outputChannel, reader)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, outputChannel, nil
}
func (external *externalApp) runCommandWait(ctx context.Context, arg ...string) (WaitAble, error) {
	wait, _, err := external.runCommand(ctx, false, arg...)
	return wait, err
}
func (external *externalApp) runCommandChan(ctx context.Context, arg ...string) (<-chan string, error) {
	_, channel, err := external.runCommand(ctx, true, arg...)
	return channel, err
}
