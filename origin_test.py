#!/usr/bin/env python3

import grpc
import os

import sys
sys.path.append("grpc_folder")
import origin_pb2
import origin_pb2_grpc

ORIGIN_IP = "127.0.0.1"
ORIGIN_PORT = "9100"

#Max size of a gRPC message is default 4MB
#So maybe do chunks of 3MB



#Need to take 'test_video.mp4' and send it over gRPC to the origin_server for processing
#Convert to bytes


# def sendMsg(video_chunk):


import hashlib
    


def generateMsgs():

    grpcMsgs = []

    #Chunk the 'test_video.mp4' into 3MB chunks and call sendMsg
    #Go until there are no more chunks

    totalFileSize = os.path.getsize("test_video.ts") 

    # numChunks = totalFileSizeMB / 3

    chunkSize =  188* 1000 # <3MB, and TS format has chunks aligned at 188 bytes each
    offset = 0
    lastOffset = -1


    with open("test_video.ts", "rb") as file:
        chunk_num = 0
        while True:
            chunk = file.read(188 * 1000)
            if not chunk:
                break
            print(f"Chunk {chunk_num} size: {len(chunk)}, md5: {hashlib.md5(chunk).hexdigest()}")
            chunk_num += 1



    with open("test_video.ts", "rb") as file:

        while(True):

            file.seek(offset)
            ptrLocation = file.tell()

            if (ptrLocation >= totalFileSize): #Reached last chunk of file
                tempChunk = file.read(totalFileSize - lastOffset)
                grpcMsgs.append(origin_pb2.ingest_video_request(
                    streamer_id = 1, #Auto assigned by the PoP in the future
                    video_format = origin_pb2.Format.MP4,
                    video_codec = origin_pb2.Codec.H264,
                    video_data = tempChunk,
                    enable_ml_censorship = False,
                    video_res = origin_pb2.Resolution.p1080,
                    frame_rate = origin_pb2.FrameRate.fps30
                ))
                break
            else:
                tempChunk = file.read(chunkSize)

        # clip = VideoFileClip(tempFileName)
        # if (clip.duration > BATCH_DURATION):
        #     fiveSecClip = clip.subclip(0, BATCH_DURATION) #Gets the first BATCH_DURATION seconds of video
        #     clip = clip.cutout(0, BATCH_DURATION)
        #     clip.write_videofile(tempFileName) #Removes batch, leaves any remainder data to be included in the next batch, overwrites file contents

                # tempClip = VideoFileClip("test_video.mp4")

                # if (tempClip.duration > )

                
                grpcMsgs.append(origin_pb2.ingest_video_request(
                    streamer_id = 1, #Auto assigned by the PoP in the future
                    video_format = origin_pb2.Format.MP4,
                    video_codec = origin_pb2.Codec.H264,
                    video_data = tempChunk,
                    enable_ml_censorship = False,
                    video_res = origin_pb2.Resolution.p1080,
                    frame_rate = origin_pb2.FrameRate.fps30
                ))

                lastOffset = offset
                offset += chunkSize

    for request in grpcMsgs:
        yield request







def main():

    originChannel = grpc.insecure_channel("%s:%s" % (ORIGIN_IP, ORIGIN_PORT))
    originStub = origin_pb2_grpc.OriginStub(originChannel)

    originResponses = originStub.ingest_video_rpc(generateMsgs())

    for originResponse in originResponses:
        print("Origin Response: %s" % originResponse)    
        




if __name__ == "__main__":
    main()