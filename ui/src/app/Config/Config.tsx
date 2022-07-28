import * as React from 'react';
import { Skeleton } from '@patternfly/react-core';
import { usePageVisibility } from 'react-page-visibility';
import { useQuery } from 'urql';
import { useEffect, useRef, useState } from 'react';
import { CodeEditor, Language } from '@patternfly/react-code-editor';

const configQuery = `{
  config {
    yaml
  }
}`;

const Config: React.FunctionComponent = () => {
  const [isPageLoading, setIsPageLoading] = useState(true);
  const pageVisible = useRef(true);
  pageVisible.current = usePageVisibility();
  const [result, reexecuteQuery] = useQuery({
    query: configQuery,
  });
  useEffect(() => {
    if (result.fetching) return;
    const timerId = setTimeout(() => {
      reexecuteQuery({ requestPolicy: 'network-only' });
    }, 1000);
    return () => clearTimeout(timerId);
  }, [result.fetching, reexecuteQuery]);
  if (!result.fetching && isPageLoading) {
    setIsPageLoading(false);
  }
  if (isPageLoading)
    return (
      <React.Fragment>
        <br />
        <Skeleton />
        <br />
        <Skeleton />
        <br />
        <Skeleton />
      </React.Fragment>
    );
  if (result.error) return <p>{JSON.stringify(result.error)}</p>;
  return (
    <React.Fragment>
      <CodeEditor
        code={result.data['config']['yaml']}
        isLineNumbersVisible={true}
        isReadOnly={true}
        language={Language.yaml}
        width="100%"
        height="100%"
        options={{
          automaticLayout: true,
          renderIndentGuides: false,
          renderWhitespace: 'none',
          tabSize: 4,
        }}
      />
    </React.Fragment>
  );
};

export { Config };
