import jmespath
import requests
import yaml
import syntest_wrapper
import syntest_pb2


class JsonPingTest(syntest_wrapper.PythonSynTest):
    """
    JsonPingTest is a test plugin that fetches a JSON response from a URL and validates using JMESPath queries.
    Example config:
    ```
    url: https://abc.com/api/v1/data
    queries:
      - query: "data[?name=='test'].value | [0]"
        expected: 100
      - query: "data[?name=='test2'].value | [0]"
        expected: 200
    ```
    """
    def __init__(self):
        self.config = {}

    def Initialise(self, test_config, context):
        c = yaml.load(test_config.config, Loader=yaml.SafeLoader)
        # check for required keys
        if 'url' not in c:
            raise KeyError("expected key: 'url' not found")
        if 'queries' not in c:
            raise KeyError("expected key: 'queries' not found")
        self.config = c
        return syntest_pb2.Empty()

    def PerformTest(self, trigger, context):

        # Fetch url with json response
        self.log(f"Fetching {self.config['url']}")
        request = requests.get(self.config['url'])
        response = request.json()

        marks = 0
        max_marks = len(self.config['queries'])

        # Validate response using JMESPath queries
        for query in self.config['queries']:
            # Validate response using JMESPath query
            self.log(f"Validating {query['query']} with {query['expected']}")
            try:
                result = jmespath.search(query['query'], response)
                if result == query['expected']:
                    self.log(f" - Passed: val={result} expected={query['expected']}")
                    marks += 1
                else:
                    self.log(f" - Failed: val={result} expected={query['expected']}")
            except Exception as e:

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