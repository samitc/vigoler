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

func (external *externalApp) runCommand(ctx context.Context, createChan bool, closeReader bool, arg ...string) (WaitAble, io.ReadCloser, <-chan string, error) {
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
			go func(outChan chan string, reader io.ReadCloser, closeReader bool) {
				if closeReader {
					defer reader.Close()
				}
				rd := bufio.NewReader(reader)
				line, err := rd.ReadString('\n')
				for ; err == nil; line, err = rd.ReadString('\n') {
					outChan <- line
				}
				if err != io.EOF {
					panic(err)
				}
				close(outChan)
			}(outputChannel, reader, closeReader)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	return cmd, streamReader, outputChannel, nil
}
func (external *externalApp) runCommandWait(ctx context.Context, arg ...string) (WaitAble, error) {
	wait, _, _, err := external.runCommand(ctx, false, true, arg...)
	return wait, err
}
func (external *externalApp) runCommandChan(ctx context.Context, arg ...string) (<-chan string, error) {
	_, _, channel, err := external.runCommand(ctx, true, true, arg...)
	return channel, err
}
