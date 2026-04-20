from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Codec(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    H264: _ClassVar[Codec]
    H265: _ClassVar[Codec]
    H266: _ClassVar[Codec]
    VP8: _ClassVar[Codec]
    VP9: _ClassVar[Codec]
    AV1: _ClassVar[Codec]

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
    HPS: _ClassVar[Format]
H264: Codec
H265: Codec
H266: Codec
VP8: Codec
VP9: Codec
AV1: Codec
p1080: Resolution
p720: Resolution
p480: Resolution
p360: Resolution
p240: Resolution
fps30: FrameRate
fps60: FrameRate
MP4: Format
HPS: Format

class ingest_video_request(_message.Message):
    __slots__ = ("streamer_id", "video_format", "video_codec", "video_data", "enable_ml_censorship", "video_res", "frame_rate")
    STREAMER_ID_FIELD_NUMBER: _ClassVar[int]
    VIDEO_FORMAT_FIELD_NUMBER: _ClassVar[int]
    VIDEO_CODEC_FIELD_NUMBER: _ClassVar[int]
    VIDEO_DATA_FIELD_NUMBER: _ClassVar[int]
    ENABLE_ML_CENSORSHIP_FIELD_NUMBER: _ClassVar[int]
    VIDEO_RES_FIELD_NUMBER: _ClassVar[int]
    FRAME_RATE_FIELD_NUMBER: _ClassVar[int]
    streamer_id: int
    video_format: Format
    video_codec: Codec
    video_data: bytes
    enable_ml_censorship: bool
    video_res: Resolution
    frame_rate: FrameRate
    def __init__(self, streamer_id: _Optional[int] = ..., video_format: _Optional[_Union[Format, str]] = ..., video_codec: _Optional[_Union[Codec, str]] = ..., video_data: _Optional[bytes] = ..., enable_ml_censorship: bool = ..., video_res: _Optional[_Union[Resolution, str]] = ..., frame_rate: _Optional[_Union[FrameRate, str]] = ...) -> None: ...

class ingest_video_response(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: bool = ...) -> None: ...
