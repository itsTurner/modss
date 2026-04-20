from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class VideoCodec(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    H264: _ClassVar[VideoCodec]
    H265: _ClassVar[VideoCodec]
    VP8: _ClassVar[VideoCodec]
    VP9: _ClassVar[VideoCodec]
    AV1: _ClassVar[VideoCodec]

class AudioCodec(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    AAC: _ClassVar[AudioCodec]
    MP3: _ClassVar[AudioCodec]

class Resolution(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    p1080: _ClassVar[Resolution]
    p720: _ClassVar[Resolution]
    p480: _ClassVar[Resolution]
    p360: _ClassVar[Resolution]
    p240: _ClassVar[Resolution]

class FrameRate(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    fps30: _ClassVar[FrameRate]
    fps60: _ClassVar[FrameRate]

class Format(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    MP4: _ClassVar[Format]
    HLS: _ClassVar[Format]
H264: VideoCodec
H265: VideoCodec
VP8: VideoCodec
VP9: VideoCodec
AV1: VideoCodec
AAC: AudioCodec
MP3: AudioCodec
p1080: Resolution
p720: Resolution
p480: Resolution
p360: Resolution
p240: Resolution
fps30: FrameRate
fps60: FrameRate
MP4: Format
HLS: Format

class ingest_video_request(_message.Message):
    __slots__ = ("streamer_id", "video_format", "video_codec", "audio_codec", "video_data", "enable_ml_censorship", "enable_watermark", "video_res", "frame_rate", "video_bitrate_mbps", "audio_bitrate_kbps")
    STREAMER_ID_FIELD_NUMBER: _ClassVar[int]
    VIDEO_FORMAT_FIELD_NUMBER: _ClassVar[int]
    VIDEO_CODEC_FIELD_NUMBER: _ClassVar[int]
    AUDIO_CODEC_FIELD_NUMBER: _ClassVar[int]
    VIDEO_DATA_FIELD_NUMBER: _ClassVar[int]
    ENABLE_ML_CENSORSHIP_FIELD_NUMBER: _ClassVar[int]
    ENABLE_WATERMARK_FIELD_NUMBER: _ClassVar[int]
    VIDEO_RES_FIELD_NUMBER: _ClassVar[int]
    FRAME_RATE_FIELD_NUMBER: _ClassVar[int]
    VIDEO_BITRATE_MBPS_FIELD_NUMBER: _ClassVar[int]
    AUDIO_BITRATE_KBPS_FIELD_NUMBER: _ClassVar[int]
    streamer_id: int
    video_format: Format
    video_codec: VideoCodec
    audio_codec: AudioCodec
    video_data: bytes
    enable_ml_censorship: bool
    enable_watermark: bool
    video_res: Resolution
    frame_rate: FrameRate
    video_bitrate_mbps: int
    audio_bitrate_kbps: int
    def __init__(self, streamer_id: _Optional[int] = ..., video_format: _Optional[_Union[Format, str]] = ..., video_codec: _Optional[_Union[VideoCodec, str]] = ..., audio_codec: _Optional[_Union[AudioCodec, str]] = ..., video_data: _Optional[bytes] = ..., enable_ml_censorship: bool = ..., enable_watermark: bool = ..., video_res: _Optional[_Union[Resolution, str]] = ..., frame_rate: _Optional[_Union[FrameRate, str]] = ..., video_bitrate_mbps: _Optional[int] = ..., audio_bitrate_kbps: _Optional[int] = ...) -> None: ...

class ingest_video_response(_message.Message):
    __slots__ = ("success", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    def __init__(self, success: bool = ..., error: _Optional[str] = ...) -> None: ...

class fetch_chunk_request(_message.Message):
    __slots__ = ("streamer_id", "chunk_id")
    STREAMER_ID_FIELD_NUMBER: _ClassVar[int]
    CHUNK_ID_FIELD_NUMBER: _ClassVar[int]
    streamer_id: int
    chunk_id: int
    def __init__(self, streamer_id: _Optional[int] = ..., chunk_id: _Optional[int] = ...) -> None: ...

class fetch_chunk_response(_message.Message):
    __slots__ = ("success", "error", "chunk_data")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    CHUNK_DATA_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    chunk_data: bytes
    def __init__(self, success: bool = ..., error: _Optional[str] = ..., chunk_data: _Optional[bytes] = ...) -> None: ...
