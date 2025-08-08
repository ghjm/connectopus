import * as React from 'react';
import { Skeleton, PageSection } from '@patternfly/react-core';
import { useQuery } from 'urql';
import { useEffect, useState } from 'react';
import { CodeEditor, Language } from '@patternfly/react-code-editor';

const configQuery = `{
  config {
    yaml
  }
}`;

const Config: React.FunctionComponent = () => {
  const [isPageLoading, setIsPageLoading] = useState(true);
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
      <PageSection isFilled>
        <br />
        <Skeleton />
        <br />
        <Skeleton />
        <br />
        <Skeleton />
      </PageSection>
    );
  if (result.error)
    return (
      <PageSection isFilled>
        <p>{JSON.stringify(result.error)}</p>
      </PageSection>
    );
  return (
    <PageSection isFilled padding={{ default: 'noPadding' }}>
      <CodeEditor
        code={result.data['config']['yaml']}
        isLineNumbersVisible={true}
        isReadOnly={true}
        language={Language.yaml}
        width="100%"
        height="sizeToFit"
        options={{
          automaticLayout: true,
          renderWhitespace: 'none',
          tabSize: 4,
          scrollbar: {
            alwaysConsumeMouseWheel: false,
          },
        }}
      />
    </PageSection>
  );
};

export { Config };
