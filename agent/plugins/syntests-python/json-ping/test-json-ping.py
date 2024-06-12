import jmespath
import requests
import yaml
import syntest_wrapper
import syntest_pb2
import re

class JsonPingTest(syntest_wrapper.PythonSynTest):
    """
    JsonPingTest is a test plugin that fetches a JSON response from a URL and validates using JMESPath queries.
    JMESPath: https://jmespath.org/
    Example config:
    ```
    url: https://api64.ipify.org/?format=json
    queries:
      - query: "ip"
        expected: "^\\d+\\.\\d+\\.\\d+\\.\\d+$"
    ```
    """
    def __init__(self):
        self.config = {}

    def Initialise(self, test_config, context):
        c = yaml.load(test_config.config, Loader=yaml.SafeLoader)
        if c is None:
            raise ValueError("Invalid yaml config")
        self.config = c
        # check for required keys
        if 'url' not in c:
            raise KeyError("expected key: 'url' not found")
        if 'queries' not in c:
            raise KeyError("expected key: 'queries' not found")
        return syntest_pb2.Empty()

    def PerformTest(self, trigger, context):

        # Fetch url with json response
        self.log(f"Fetching {self.config['url']}")
        request = requests.get(self.config['url'])
        response = request.json()

        marks = 0
        max_marks = len(self.config['queries'])  # Max marks is however many queries we have

        # Validate response using JMESPath queries
        for query in self.config['queries']:
            self.log(f"Validating {query['query']} with {query['expected']}")
            try:
                result = jmespath.search(query['query'], response)
                match = re.search(query['expected'], result)
                if match is not None:
                    self.log(f" - Passed: val={result} expected={query['expected']}")
                    marks += 1
                else:
                    self.log(f" - Failed: val={result} expected={query['expected']}")
            except Exception as e:
                self.log(f" - Failed (Exception): {str(e)}")

        return syntest_pb2.TestResult(
                marks=marks,
                maxMarks=max_marks,
                details={}
        )

    def Finish(self, request, context):
        self.log("Finishing...")
        return syntest_pb2.Empty()


if __name__ == '__main__':
    JsonPingTest().serve()
