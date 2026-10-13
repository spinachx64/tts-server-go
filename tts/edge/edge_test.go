package edge

import (
	"os"
	"testing"
	"time"
)

func TestEdgeApi(t *testing.T) {
	if os.Getenv("TTS_INTEGRATION_TEST") != "1" {
		t.Skip("设置 TTS_INTEGRATION_TEST=1 启用微软在线接口测试")
	}
	ssml := `<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts" xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="en-US">          <voice name="zh-CN-XiaoxiaoNeural">              <prosody rate="0%" pitch="0%">                  如果喜欢这个项目的话请点个 Star 吧。              </prosody >          </voice >        </speak >`
	for _, format := range []string{"audio-24khz-48kbitrate-mono-mp3", "webm-24khz-16bit-mono-opus"} {
		t.Run(format, func(t *testing.T) {
			tts := &TTS{}
			defer tts.CloseConn()
			audioData, err := tts.GetAudio(ssml, format)
			if err != nil {
				t.Fatal(err)
			}
			if len(audioData) == 0 {
				t.Fatal("微软接口未返回音频数据")
			}
			if err := os.WriteFile(t.TempDir()+"/audio", audioData, 0600); err != nil {
				t.Fatal(err)
			}
		})
	}
}

//func TestEdgeApiRetry(t *testing.T) {
//	ssml := `错误ssml<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts" xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="en-US"><voice name="zh-CN-XiaoxiaoNeural"><prosody rate="200%" pitch="+0Hz">　　半年后一天，苏浩再次尝试控制身上的血气运动，原本以为会一如既往般毫无动静，没想到意识操控的那部分血气竟然往控制方向移动了一丝。就是这一丝移动，让苏浩欣喜若狂。</prosody></voice></speak>`
//	_, err := GetAudioForRetry(ssml, "audio-24khz-48kbitrate-mono-mp3", 3)
//	if err != nil {
//		t.Fatal(err)
//		return
//	}
//}

//func TestCloseConn(t *testing.T) {
//	go func() {
//		time.Sleep(time.Second * 3)
//		CloseConn()
//	}()
//	ssml := `<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts" xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="en-US"><voice name="zh-CN-XiaoxiaoNeural"><prosody rate="200%" pitch="+0Hz">　　半年后一天，苏浩再次尝试控制身上的血气运动，原本以为会一如既往般毫无动静，没想到意识操控的那部分血气竟然往控制方向移动了一丝。就是这一丝移动，让苏浩欣喜若狂。</prosody></voice></speak>`
//	for i := 0; i < 3; i++ {
//		_, err := GetAudio(ssml, "audio-24khz-48kbitrate-mono-mp3")
//		if err != nil {
//			t.Fatal(err)
//		}
//	}
//
//}

func TestGenerateSecMSGEC(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		want    string
	}{
		{0, "7ECB79D14E3AA576D2D79E6D487A1388156D91E614B1BE11C64226A29BC8DD8C"},
		{299, "7ECB79D14E3AA576D2D79E6D487A1388156D91E614B1BE11C64226A29BC8DD8C"},
		{300, "ED93F5AAE06C01D88654A1831CAC424F7BE4878E9D5850F7F66DDCBDD7ED95B9"},
	} {
		if got := generateSecMSGEC(time.Unix(tc.seconds, 0)); got != tc.want {
			t.Errorf("timestamp %d: got %s, want %s", tc.seconds, got, tc.want)
		}
	}
}
