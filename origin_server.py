#!/usr/bin/env python3

import grpc
from concurrent import futures

import subprocess

import av
import ffmpeg
from moviepy.video.io.ffmpeg_tools import ffmpeg_extract_subclip
from moviepy import * #Built on top of ffmpeg, works with basically any codec
import os
import time
import hashlib

import sys
sys.path.append("grpc_folder")
import grpc_folder.origin_pb2 as origin_pb2
import grpc_folder.origin_pb2_grpc as origin_pb2_grpc



MAX_WORKER_THREADS = 10 
BATCH_DURATION = 5 #5 seconds


class VideoProcessor():


    def cleanupOldChunks(self):
        pass
        #This is the callback function for a thread
        #Need to include a while loop here, sleep 1 second to reduce power usage
        #Check all files
        #Check if last access time for the chunk was > 10*BATCH_DURATION
            #If so, delete file
            #If not, ignore



    def transcodeHLS(self, streamer_id, video_data, video_codec, video_res, frame_rate, audio_codec, video_bitrate_mbps, audio_bitrate_kbps):
        #Use the params passed in to do different ffmpeg operations on the data.
        #For HLS format, assume input is the split .ts files containing individual video chunks

        #Setup the codec param for ffmpeg
        if video_codec == "H264":
            mpegVideoCodec = "-c:v libx264"
        elif video_codec == "H265":
            mpegVideoCodec = "-c:v libx265"
        elif video_codec == "VP8":
            mpegVideoCodec = "-c:v libvpx"
        elif video_codec == "VP9":
            mpegVideoCodec = "-c:v libvpx-vp9"

        #Setup the resolution param for ffmpeg
        if video_res == "p1080":
            #ffmpeg auto calculates the width when the width is negative.
            #'-2' makes it work with the default H264 codec (makes sure it's an even number calculated)
            mpegRes = "-vf scale=-2:1080" 
        elif video_res == "p720":
            mpegRes = "-vf scale=-2:720"
        elif video_res == "p480":
            mpegRes = "-vf scale=-2:480"
        elif video_res == "p360":
            mpegRes = "-vf scale=-2:360"
        elif video_res == "p240":
            mpegRes = "-vf scale=-2:240"


        if frame_rate == "fps30":
            mpegFR = "-filter:v fps=30"
        elif frame_rate == "fps60":
            mpegFR = "-filter:v fps=60"

        
        if audio_codec == "AAC":
            mpegAudioCodec = "-c:a aac"
        elif audio_codec == "MP3":
            mpegAudioCodec = "-c:a libmp3lame"


        mpegVideoBR = "-b:v %dM" % (video_bitrate_mbps)
        mpegAudioBR = "-b:a %dk" % (audio_bitrate_kbps)

        #Store the initial input file (pre-processing) to a temporary file
        tempInputFile = "HLSTranscodeInput_%s.ts" % (streamer_id)
        with open(tempInputFile, "wb") as tempFile:
            tempFile.write(video_data)

        tempOutputFile = "HLSTranscodeOutput_%s.ts" % (streamer_id)

        subprocess.run(
            [
                "ffmpeg",
                "-y", #Automatically say yes to overwrites
                "-i",
                tempInputFile,
                #Transcoding options
                mpegVideoCodec,
                mpegAudioCodec,
                mpegVideoBR,
                mpegAudioBR,
                mpegRes,
                mpegFR,
                #Output file
                tempOutputFile
            ]
        )
        #Get output to send on to the next node in the network
        transcodedDataOut = None
        with open(tempOutputFile, "rb") as tempFile:
            transcodedDataOut = tempFile.read()

        return transcodedDataOut
        

        

    def mlCensorship(self, video_data):
        pass
        #Placeholder: Delay representing some additional video processing of a machine learning algorithm that can censor
        # video frames in video chunks as they come in.
        time.sleep(0.1)
        return video_data
    

    def watermarkProcessing(self, video_data):
        pass
        #Placeholder: Delay representing additional per-frame video processing to add a watermark.
        time.sleep(0.02)
        return video_data




    def process_video(self, streamer_id: int, 
                      video_format: int, 
                      video_codec: int, 
                      audio_codec: int,
                      video_data: bytes,
                      enable_ml_censorship: bool, 
                      enable_watermark: bool,
                      video_res: int, 
                      frame_rate: int,
                      video_bitrate_mbps: int,
                      audio_bitrate_kbps: int
                      ):
        
        #Check video format, call corresponding function
        video_format = origin_pb2.Format.Name(video_format)
        video_codec = origin_pb2.VideoCodec.Name(video_codec)
        audio_codec = origin_pb2.AudioCodec.Name(audio_codec)
        video_res = origin_pb2.Resolution.Name(video_res)
        frame_rate = origin_pb2.FrameRate.Name(frame_rate)

        #Batch into 5 sec clips first, then do additional processing as needed.
        if (video_format == "MP4"):
            pass
            status = None
            outputChunk = None

        elif (video_format == "HLS"):
            #Need to use ffmpeg to convert to the desired codec, resolution, bitrate, and framerate
            outputChunk = self.transcodeHLS(
                streamer_id, 
                video_data, 
                video_codec, 
                video_res, 
                frame_rate, 
                audio_codec, 
                video_bitrate_mbps, 
                audio_bitrate_kbps)

        if (enable_ml_censorship):
            outputChunk = self.mlCensorship(outputChunk)

        if (enable_watermark):
            outputChunk = self.watermarkProcessing(outputChunk)

        return status
    

    def fetch_chunk(self, streamer_id: int, chunk_id: int):
        #Check if file exists in memory on this Origin server
        #If it does, send the chunk to the requesting PoP
        #If it does not, send an error indicating the chunk was not found
        chunkData = None
        error = None

        chunkFileName = "buffer_%d_chunk_%s" % (streamer_id, chunk_id)
        if os.path.isfile(chunkFileName):
            with open(chunkFileName, 'rb') as tempFile:
                chunkData = tempFile.read()
            return { "success" : False, "chunk_data" : chunkData }
        else:
            error = "Chunk not found"
            return { "success" : False, "error" : error }
        
        


class OriginServicer(origin_pb2_grpc.OriginServicer):

    def __init__(self, video_processor: VideoProcessor):
        self.video_processor = video_processor


    def ingest_video_rpc(self, requests, context):
        for request in requests:
            servicerResponse = self.video_processor.process_video(
                request.streamer_id,
                request.video_format,
                request.video_codec,
                request.video_data,
                request.enable_ml_censorship,
                request.video_res,
                request.frame_rate
            )
            if("error" in servicerResponse):
                tempError = servicerResponse["error"]
            else:
                tempError = ""
            yield origin_pb2.ingest_video_response(
                success = bool(servicerResponse["success"])
            )

    def fetch_chunk_rpc(self, requests, context):
        for request in requests:
            servicerResponse = self.video_processor.fetch_chunk(
                request.streamer_id,
                request.chunk_id
            )
            if("error" in servicerResponse):
                tempError = str(servicerResponse["error"])
                chunkData = ""
                success = False
            else:
                tempError = ""
                chunkData = servicerResponse["chunk_data"]
                success = True
            yield origin_pb2.fetch_chunk_response(
                success,
                tempError,
                chunkData
            )



def tempCleanup():

    streamerBufferFile = "buffer_1.ts"
    remainderBufferFile = "buffer_cut1.ts" #For remainders after 5 seconds are cut out
    fiveSecBufferFile = "fivesecbuffer_1.ts"

    with open(streamerBufferFile, 'w') as tempFile:
            tempFile.write("")

    with open(remainderBufferFile, 'w') as tempFile:
            tempFile.write("")

    with open(fiveSecBufferFile, 'w') as tempFile:
            tempFile.write("")






def main():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers= MAX_WORKER_THREADS))

    videoProcessor = VideoProcessor()

    tempCleanup()

    origin_pb2_grpc.add_OriginServicer_to_server(OriginServicer(videoProcessor), server)
    server.add_insecure_port("[::]:9100") #TODO: Temporary, fix***************
    server.start() #Non-blocking
    server.wait_for_termination()



if __name__ == "__main__":
    main()