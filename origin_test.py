#!/usr/bin/env python3

import grpc
import os

import hashlib

import sys
sys.path.append("grpc_folder")
import grpc_folder.origin_pb2 as origin_pb2
import grpc_folder.origin_pb2_grpc as origin_pb2_grpc

ORIGIN_IP = "127.0.0.1"
ORIGIN_PORT = "9100"

MAX_MESSAGE_LENGTH = 8388608



def generateMsgs():

    #Basically just send through the pre-generated .ts files (which are already split on keyframes ~5 seconds each)

    grpcMsgs = []
    numChunks = 26
    inputVideoName = "tempInput/test_video"

    for i in range(0, numChunks):
        tempFileName = inputVideoName + str(i) + ".ts"
        inputChunkContents = None

        #Read contents of the .ts file and craft grpc message request
        with open(tempFileName, 'rb') as tempFile:
            inputChunkContents = tempFile.read()

        grpcMsgs.append(origin_pb2.ingest_video_request(
            streamer_id= 1,
            video_format= origin_pb2.Format.HLS,
            video_codec= origin_pb2.VideoCodec.H264,
            audio_codec= origin_pb2.AudioCodec.AAC,
            video_data= bytes(inputChunkContents),
            enable_ml_censorship= True,
            enable_watermark= True,
            video_res= origin_pb2.Resolution.p480,
            frame_rate= origin_pb2.FrameRate.fps30,
            video_bitrate_mbps= 3,
            audio_bitrate_kbps= 48
        ))

    
    for request in grpcMsgs:
        yield request




def fetchMsgs():

    fetchMsgs = []
    testMsgsToFetch = [3,4,5,6,9]

    for chunkToFetch in testMsgsToFetch:
        fetchMsgs.append(origin_pb2.fetch_chunk_request(
            streamer_id= 1,
            chunk_id= chunkToFetch
        ))

    for request in fetchMsgs:
        yield request



def main():

    originChannel = grpc.insecure_channel("%s:%s" % (ORIGIN_IP, ORIGIN_PORT),
                                          options = [
                                                ('grpc.max_send_message_length', MAX_MESSAGE_LENGTH),
                                                ('grpc.max_receive_message_length', MAX_MESSAGE_LENGTH)      
                                            ])
                                          
    

    originStub = origin_pb2_grpc.OriginStub(originChannel)

    originResponses = originStub.ingest_video_rpc(generateMsgs())
    for originResponse in originResponses:
        print("Origin Response: %s" % originResponse)    


    #Now test the request RPC

    originResponses = originStub.fetch_chunk_rpc(fetchMsgs())
    for originResponse in originResponses:
        #Save / play the received chunks

        fetchedChunkData = originResponse.chunk_data
        fetchedChunkID = originResponse.chunk_id
        fetchedStreamerID = originResponse.streamer_id

        with open("tempFetched/fetched%d_%d.ts" % (int(fetchedStreamerID), int(fetchedChunkID)), 'wb') as tempFile:
            tempFile.write(fetchedChunkData)
        
        #ffmpeg -y -i fetched1_3.ts -c:v libx264 -c:a aac -b:v 3M -b:a 48k -filter:v fps=30 -vf scale=-2:480 test3.mp4

        




if __name__ == "__main__":
    main()