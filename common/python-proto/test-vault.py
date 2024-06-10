import os
import sys
import time

import hvac
import logging
from concurrent import futures
import grpc
import syntest_pb2
import syntest_pb2_grpc
from grpc_health.v1.health import HealthServicer
from grpc_health.v1 import health_pb2, health_pb2_grpc
import yaml

def get_failed_result(details):
    return syntest_pb2.SynTestResult(
        marks=0,
        max_marks=1,
        details=details
    )

class VaultTest(syntest_pb2_grpc.SynTestPluginServicer):
    """Implementation of SynTest service."""

    def Initialise(self, request, context):
        prints("Initialising...")
        c = yaml.load(request.config, Loader=yaml.SafeLoader)
        self.config = c
        return syntest_pb2.Empty()

    def PerformTest(self, request, context):
        prints("Testing...")
        prints("This is a example python test")
        prints("Config: %s", self.config)
        return syntest_pb2.TestResult(
                marks=1,
                maxMarks=1,
                details={}
        )

    def Finish(self, request, context):
        prints("Finishing...")
        return syntest_pb2.Empty()



def serve():
    # We need to build a health service to work with go-plugin
    health = HealthServicer()
    health.set("plugin", health_pb2.HealthCheckResponse.ServingStatus.Value('SERVING'))

    # Start the server.
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    syntest_pb2_grpc.add_SynTestPluginServicer_to_server(VaultTest(), server)
    health_pb2_grpc.add_HealthServicer_to_server(health, server)
    serve_port = server.add_insecure_port('127.0.0.1:0')

    server.start()

    # Output important info on stdout for plugin setup
    print("1|1|tcp|127.0.0.1:" + str(serve_port) + "|grpc")
    sys.stdout.flush()

    # server.start() will not block, so a sleep-loop is added to keep alive
    try:
        while True:
            time.sleep(60 * 60 * 24)
    except KeyboardInterrupt:
        server.stop(0)

def prints(*args):
    print(*args, file=sys.stderr)


if __name__ == '__main__':
    serve()