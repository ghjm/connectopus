import * as React from 'react';
import { NavLink, useLocation, useHistory } from 'react-router-dom';
import {
  Nav,
  NavList,
  NavItem,
  NavExpandable,
  Page,
  PageHeader,
  PageSidebar,
  SkipToContent,
  PageHeaderTools,
  ContextSelector,
  ContextSelectorItem,
  Flex,
  Label,
  FlexItem,
} from '@patternfly/react-core';
import { routes, IAppRoute, IAppRouteGroup } from '@app/routes';
import logo from '@app/images/connectopus.png';
import nodeLogo from '@app/images/node.png';
import { Client, createClient, Provider, useQuery } from 'urql';
import { createContext, useEffect } from 'react';
import GitHubButton from 'react-github-btn';

interface IAppLayout {
  children: React.ReactNode;
}

interface IAppContext {
  myNodeState: [string, React.Dispatch<React.SetStateAction<string>>];
  activeNodeState: [string, React.Dispatch<React.SetStateAction<string>>];
}

interface IAppContent extends IAppLayout, IAppContext {
  mainClient: Client;
}

const AppContext = createContext<IAppContext | null>(null);

const statusQuery = `{
  status {
    name
    nodes {
      name
      addr
    }
  }
}`;

const AppLayoutContent: React.FunctionComponent<IAppContent> = ({
  myNodeState,
  activeNodeState,
  mainClient,
  children,
}) => {
  const [myNode, setMyNode] = myNodeState;
  const [activeNode, setActiveNode] = activeNodeState;
  const [isNavOpen, setIsNavOpen] = React.useState(true);
  const [isMobileView, setIsMobileView] = React.useState(true);
  const [isNavOpenMobile, setIsNavOpenMobile] = React.useState(false);
  const [contextSelectorSelected, setContextSelectorSelected] = React.useState(activeNode);
  const [isContextSelectorOpen, setIsContextSelectorOpen] = React.useState(false);
  const [searchText, setSearchText] = React.useState('');
  const [searchValue, setSearchValue] = React.useState('');
  const [result, reexecuteQuery] = useQuery({
    query: statusQuery,
  });
  useEffect(() => {
    if (result.fetching) return;
    if (!('data' in result)) return;
    if (result.data === undefined) return;
    if (!('status' in result.data)) return;
    if (!('name' in result.data.status)) return;
    const sn = result['data']['status']['name'];
    if (sn !== myNode) {
      setMyNode(sn);
      sessionStorage.setItem('myNode', sn);
    }
    const timerId = setTimeout(() => {
      reexecuteQuery({ requestPolicy: 'network-only' });
    }, 1000);
    return () => clearTimeout(timerId);
  }, [result, reexecuteQuery, myNode, setMyNode]);

  const onNavToggleMobile = () => {
    setIsNavOpenMobile(!isNavOpenMobile);
  };
  const navMobileClose = () => {
    setIsNavOpenMobile(false);
  };
  const onNavToggle = () => {
    setIsNavOpen(!isNavOpen);
  };
  const onPageResize = (props: { mobileView: boolean; windowSize: number }) => {
    setIsMobileView(props.mobileView);
  };

  function LogoImg() {
    const history = useHistory();
    function handleClick() {
      history.push('/');
    }
    return <img src={logo} onClick={handleClick} alt="Connectopus Logo" />;
  }

  const onToggle = (event: unknown, isOpen: boolean) => {
    setIsContextSelectorOpen(isOpen);
  };

  const onSearchInputChange = (value: string) => {
    setSearchValue(value);
  };

  const onSearchButtonClick = () => {
    setSearchText(searchValue);
  };

  const onSelect = (event: unknown, value: React.ReactNode) => {
    if (value !== undefined && value !== null) {
      const newNode = value.toString();
      setActiveNode(newNode);
      setContextSelectorSelected(newNode);
      sessionStorage.setItem('activeNode', newNode);
      setIsContextSelectorOpen(false);
    }
  };

  const getContextSelectors = () => {
    if (!('data' in result && result.data !== undefined && 'status' in result.data)) {
      return null;
    }
    return result.data['status']['nodes']
      .filter((node) => {
        if (searchText === '') {
          return true;
        }
        return node.name.includes(searchText);
      })
      .map((node) => <ContextSelectorItem key={node.name}>{`${node.name}`}</ContextSelectorItem>);
  };

  const Selector = (
    <ContextSelector
      toggleText={contextSelectorSelected}
      searchInputValue={searchValue}
      isOpen={isContextSelectorOpen}
      onToggle={onToggle}
      onSelect={onSelect}
      onSearchInputChange={onSearchInputChange}
      onSearchButtonClick={onSearchButtonClick}
    >
      {getContextSelectors()}
    </ContextSelector>
  );

  const Header = (
    <PageHeader
      logo={<LogoImg />}
      showNavToggle
      isNavOpen={isNavOpen}
      onNavToggle={isMobileView ? onNavToggleMobile : onNavToggle}
      headerTools={<PageHeaderTools>{Selector}</PageHeaderTools>}
    />
  );

  const location = useLocation();

  const renderNavItem = (route: IAppRoute, index: number) => (
    <NavItem
      key={`${route.label}-${index}`}
      id={`${route.label}-${index}`}
      isActive={route.path === location.pathname}
      onClick={navMobileClose}
    >
      <NavLink exact={route.exact} to={route.path}>
        {route.label}
      </NavLink>
    </NavItem>
  );

  const renderNavGroup = (group: IAppRouteGroup, groupIndex: number) => (
    <NavExpandable
      key={`${group.label}-${groupIndex}`}
      id={`${group.label}-${groupIndex}`}
      title={group.label}
      isActive={group.routes.some((route) => route.path === location.pathname)}
    >
      {group.routes.map((route, idx) => route.label && renderNavItem(route, idx))}
    </NavExpandable>
  );

  const NavFooter = (
    <Flex direction={{ default: 'column' }} cellSpacing={20}>
      <FlexItem alignSelf={{ default: 'alignSelfCenter' }}>
        <Label
          color="grey"
          icon={<img src={nodeLogo} alt={'logo'} width={20} />}
          isCompact={true}
          href={'https://github.com/ghjm/connectopus'}
          style={{ backgroundColor: '#8A8D90' }}
        >
          View on GitHub
        </Label>
      </FlexItem>
      <FlexItem alignSelf={{ default: 'alignSelfCenter' }}>
        <Flex>
          <FlexItem>
            <GitHubButton
              href="https://github.com/ghjm/connectopus/discussions"
              aria-label="Discuss ghjm/connectopus on GitHub"
            >
              Discuss
            </GitHubButton>
          </FlexItem>
          <FlexItem>
            <GitHubButton
              href="https://github.com/ghjm/connectopus"
              data-icon="octicon-star"
              aria-label="Star ghjm/connectopus on GitHub"
            >
              Star
            </GitHubButton>
          </FlexItem>
          <FlexItem>
            <GitHubButton
              href="https://github.com/ghjm/connectopus/issues"
              data-icon="octicon-issue-opened"
              aria-label="Issue ghjm/connectopus on GitHub"
            >
              Issue
            </GitHubButton>
          </FlexItem>
        </Flex>
      </FlexItem>
    </Flex>
  );

  const Navigation = (
    <Flex direction={{ default: 'column' }} flexWrap={{ default: 'nowrap' }}>
      <FlexItem grow={{ default: 'grow' }}>
        <Nav id="nav-primary-simple" theme="dark">
          <NavList id="nav-list-simple">
            {routes.map(
              (route, idx) => route.label && (!route.routes ? renderNavItem(route, idx) : renderNavGroup(route, idx))
            )}
          </NavList>
        </Nav>
      </FlexItem>
      <FlexItem className="pf-u-px-md pf-u-px-lg-on-xl pf-u-color-light-200 pf-u-font-size-sm">{NavFooter}</FlexItem>
    </Flex>
  );

  const Sidebar = <PageSidebar theme="dark" nav={Navigation} isNavOpen={isMobileView ? isNavOpenMobile : isNavOpen} />;

  const pageId = 'primary-app-container';

  const PageSkipToContent = (
    <SkipToContent
      onClick={(event) => {
        event.preventDefault();
        const primaryContentContainer = document.getElementById(pageId);
        primaryContentContainer && primaryContentContainer.focus();
      }}
      href={`#${pageId}`}
    >
      Skip to Content
    </SkipToContent>
  );
  return (
    <Page
      mainContainerId={pageId}
      header={Header}
      sidebar={Sidebar}
      onPageResize={onPageResize}
      skipToContent={PageSkipToContent}
    >
      <Provider value={mainClient}>{children}</Provider>
    </Page>
  );
};

const urlFromActiveNode = (myNode, activeNode: string): string => {
  if (activeNode === '' || activeNode === myNode) {
    return '/query';
  } else {
    return `/proxy/${activeNode}/query`;
  }
};

const getInitialActiveNode = (): string => {
  return sessionStorage.getItem('activeNode') || '';
};

const getInitialMyNode = (): string => {
  return sessionStorage.getItem('myNode') || '';
};

const AppLayout: React.FunctionComponent<IAppLayout> = ({ children }) => {
  const [activeNode, setActiveNode] = React.useState(getInitialActiveNode());
  const [myNode, setMyNode] = React.useState(getInitialMyNode());
  const [client, setClient] = React.useState<Client>(createClient({ url: urlFromActiveNode(myNode, activeNode) }));
  React.useEffect(() => {
    setClient(createClient({ url: urlFromActiveNode(myNode, activeNode) }));
  }, [myNode, activeNode]);
  const [topClient, setTopClient] = React.useState<Client>(
    createClient({ url: urlFromActiveNode(myNode, activeNode) })
  );
  React.useEffect(() => {
    setTopClient(createClient({ url: '/query' }));
  }, []);

  return (
    <AppContext.Provider value={{ myNodeState: [myNode, setMyNode], activeNodeState: [activeNode, setActiveNode] }}>
      <Provider value={topClient}>
        <AppLayoutContent
          myNodeState={[myNode, setMyNode]}
          activeNodeState={[activeNode, setActiveNode]}
          mainClient={client}
        >
          {children}
        </AppLayoutContent>
      </Provider>
    </AppContext.Provider>
  );
};

export { AppLayout, AppContext };
