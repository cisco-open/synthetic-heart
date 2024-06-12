import yaml
import syntest_wrapper
import syntest_pb2

class {{TestName}}Test(syntest_wrapper.PythonSynTest):
    """
    {{TestName}}Test is a test plugin that ...
    Example config:
    ```
    <Give an example config here>
    ```
    """
    def __init__(self):
        self.config = {}

    def Initialise(self, test_config, context):
        """
        Initialise your test here. This function will be called before performing the test.
        """
        self.log("Initialising...")
        return syntest_pb2.Empty()

    def PerformTest(self, trigger, context):
        """
        Perform the test here and return the result.
        """
        self.log("Performing Test...") # Dont use print(), use self.log() as stdout is used for rpc communication
        return syntest_pb2.TestResult(
                marks=0,
                maxMarks=1,
                details={}
        )

    def Finish(self, request, context):
        """
        Perform any cleanup after a test.
        """
        self.log("Finishing...")
        return syntest_pb2.Empty()


if __name__ == '__main__':
    {{TestName}}Test().serve()
