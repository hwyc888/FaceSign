package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func cameraWebRTCDiagnosticChecks(ctx context.Context, camera store.Camera) []cameraTestCheck {
	if camera.Kind != "network" || strings.ToLower(strings.TrimSpace(camera.Protocol)) != "rtsp" {
		return nil
	}

	checks := []cameraTestCheck{cameraEndpointConsistencyCheck(camera)}

	probeCtx, cancel := context.WithTimeout(ctx, cameraWebRTCProbeTimeout(camera)+4*time.Second)
	source, err := resolveRTSPSourceForPurpose(probeCtx, camera, false, "preview")
	cancel()
	if err != nil {
		checks = append(checks,
			cameraTestCheckItem("RTSP 101预览源", "error", err.Error()),
			cameraTestCheckItem("视频编码", "pending", "RTSP源未解析，无法确认编码"),
			cameraTestCheckItem("Intel QSV", "pending", "RTSP源未解析，暂不执行真实转码测试"),
		)
		return checks
	}

	codecLabel := strings.ToUpper(source.Codec)
	if source.Codec == "hevc" {
		codecLabel = "H.265/HEVC"
	} else if source.Codec == "h264" {
		codecLabel = "H.264"
	}
	checks = append(checks,
		cameraTestCheckItem("RTSP 101预览源", "ok",
			fmt.Sprintf("%s · %s · %s", source.Label, strings.ToUpper(source.Transport), redactRTSPURL(source.URL))),
	)

	codecStatus := "ok"
	codecMessage := "实际检测：" + codecLabel
	if source.Codec != "hevc" {
		codecStatus = "error"
		codecMessage += "；要恢复“WebRTC H.265→H.264硬件转码(Intel)”，101主码流必须实际检测为 H.265/HEVC"
	}
	checks = append(checks, cameraTestCheckItem("视频编码", codecStatus, codecMessage))

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		checks = append(checks, cameraTestCheckItem("Intel QSV", "error", err.Error()))
		return checks
	}

	qsvCtx, qsvCancel := context.WithTimeout(ctx, 3*time.Second)
	err = probeWebRTCH264Encoder(qsvCtx, ffmpegPath, "h264_qsv")
	qsvCancel()
	if err != nil {
		checks = append(checks, cameraTestCheckItem("Intel QSV编码器", "error", compactCameraDiagnostic(err.Error())))
		return checks
	}
	checks = append(checks, cameraTestCheckItem("Intel QSV编码器", "ok", "h264_qsv 合成画面编码测试通过"))

	if source.Codec != "hevc" {
		checks = append(checks, cameraTestCheckItem("H.265→Intel QSV真实转码", "pending", "当前101不是H.265/HEVC，因此未执行"))
		return checks
	}

	transcodeCtx, transcodeCancel := context.WithTimeout(ctx, 10*time.Second)
	err = probeCameraH265QSVTranscode(transcodeCtx, camera, source, ffmpegPath)
	transcodeCancel()
	if err != nil {
		checks = append(checks, cameraTestCheckItem("H.265→Intel QSV真实转码", "error", compactCameraDiagnostic(err.Error())))
		return checks
	}
	checks = append(checks, cameraTestCheckItem("H.265→Intel QSV真实转码", "ok", "已从该摄像头101实际读取H.265并由h264_qsv成功编码2帧"))
	return checks
}

func cameraEndpointConsistencyCheck(camera store.Camera) cameraTestCheck {
	streamHost := cameraURLHostname(camera.StreamURL)
	snapshotHost := cameraURLHostname(camera.SnapshotURL)
	if streamHost == "" || snapshotHost == "" {
		return cameraTestCheckItem("摄像头地址一致性", "pending", "RTSP或Snapshot地址缺少主机信息")
	}
	if strings.EqualFold(streamHost, snapshotHost) {
		return cameraTestCheckItem("摄像头地址一致性", "ok", "RTSP与Snapshot均指向 "+streamHost)
	}
	return cameraTestCheckItem("摄像头地址一致性", "error",
		fmt.Sprintf("RTSP指向 %s，但Snapshot指向 %s；WebRTC失败后会显示另一台摄像头的回退画面", streamHost, snapshotHost))
}

func cameraURLHostname(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func probeCameraH265QSVTranscode(ctx context.Context, camera store.Camera, source resolvedRTSPSource, ffmpegPath string) error {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-max_delay", "500000",
		"-rtsp_transport", source.Transport,
		"-timeout", strconv.FormatInt(timeout.Microseconds(), 10),
		"-i", source.URL,
		"-map", "0:v:0",
		"-frames:v", "2",
		"-an",
		"-sn",
		"-dn",
	}
	args = append(args, webRTCH264OutputArgs(camera, source, webRTCH264Encoder{
		Name: "h264_qsv",
		Mode: "WebRTC H.265→H.264硬件转码(Intel)",
	})...)
	args = append(args, "-f", "h264", "-")

	command := exec.CommandContext(ctx, ffmpegPath, args...)
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("真实转码超时或被取消: %w", ctx.Err())
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		detail = sanitizeRTSPDiagnostic(detail, camera, source.URL)
		return fmt.Errorf("真实转码失败: %s", detail)
	}
	return nil
}

func compactCameraDiagnostic(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "未知错误"
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if len(line) > 220 {
			return line[:220] + "…"
		}
		return line
	}
	return text
}
