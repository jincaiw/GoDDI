/** The global namespace for the app */
declare namespace App {
  /** Theme namespace */
  namespace Theme {
    type ColorPaletteNumber = import('@sa/color').ColorPaletteNumber;

    /** Theme setting */
    interface ThemeSetting {
      /** Theme scheme */
      themeScheme: UnionKey.ThemeScheme;
      /** grayscale mode */
      grayscale: boolean;
      /** colour weakness mode */
      colourWeakness: boolean;
      /** Whether to recommend color */
      recommendColor: boolean;
      /** Theme color */
      themeColor: string;
      /** Theme radius */
      themeRadius: number;
      /** Other color */
      otherColor: OtherColor;
      /** Whether info color is followed by the primary color */
      isInfoFollowPrimary: boolean;
      /** Layout */
      layout: {
        /** Layout mode */
        mode: UnionKey.ThemeLayoutMode;
        /** Scroll mode */
        scrollMode: UnionKey.ThemeScrollMode;
      };
      /** Page */
      page: {
        /** Whether to show the page transition */
        animate: boolean;
        /** Page animate mode */
        animateMode: UnionKey.ThemePageAnimateMode;
      };
      /** Header */
      header: {
        /** Header height */
        height: number;
        /** Header breadcrumb */
        breadcrumb: {
          /** Whether to show the breadcrumb */
          visible: boolean;
          /** Whether to show the breadcrumb icon */
          showIcon: boolean;
        };
        /** Multilingual */
        multilingual: {
          /** Whether to show the multilingual */
          visible: boolean;
        };
        globalSearch: {
          /** Whether to show the GlobalSearch */
          visible: boolean;
        };
      };
      /** Tab */
      tab: {
        /** Whether to show the tab */
        visible: boolean;
        /**
         * Whether to cache the tab
         *
         * If cache, the tabs will get from the local storage when the page is refreshed
         */
        cache: boolean;
        /** Tab height */
        height: number;
        /** Tab mode */
        mode: UnionKey.ThemeTabMode;
        /** Whether to close tab by middle click */
        closeTabByMiddleClick: boolean;
      };
      /** Fixed header and tab */
      fixedHeaderAndTab: boolean;
      /** Sider */
      sider: {
        /** Inverted sider */
        inverted: boolean;
        /** Sider width */
        width: number;
        /** Collapsed sider width */
        collapsedWidth: number;
        /** Sider width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or 'top-hybrid-header-first' */
        mixWidth: number;
        /**
         * Collapsed sider width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or
         * 'top-hybrid-header-first'
         */
        mixCollapsedWidth: number;
        /** Child menu width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or 'top-hybrid-header-first' */
        mixChildMenuWidth: number;
        /** Whether to auto select the first submenu */
        autoSelectFirstMenu: boolean;
      };
      /** Footer */
      footer: {
        /** Whether to show the footer */
        visible: boolean;
        /** Whether fixed the footer */
        fixed: boolean;
        /** Footer height */
        height: number;
        /**
         * Whether float the footer to the right when the layout is 'top-hybrid-sidebar-first' or
         * 'top-hybrid-header-first'
         */
        right: boolean;
      };
      /** Watermark */
      watermark: {
        /** Whether to show the watermark */
        visible: boolean;
        /** Watermark text */
        text: string;
        /** Whether to use user name as watermark text */
        enableUserName: boolean;
        /** Whether to use current time as watermark text */
        enableTime: boolean;
        /** Time format for watermark text */
        timeFormat: string;
      };
      /** define some theme settings tokens, will transform to css variables */
      tokens: {
        light: ThemeSettingToken;
        dark?: {
          [K in keyof ThemeSettingToken]?: Partial<ThemeSettingToken[K]>;
        };
      };
    }

    interface OtherColor {
      info: string;
      success: string;
      warning: string;
      error: string;
    }

    interface ThemeColor extends OtherColor {
      primary: string;
    }

    type ThemeColorKey = keyof ThemeColor;

    type ThemePaletteColor = {
      [key in ThemeColorKey | `${ThemeColorKey}-${ColorPaletteNumber}`]: string;
    };

    type BaseToken = Record<string, Record<string, string>>;

    interface ThemeSettingTokenColor {
      /** the progress bar color, if not set, will use the primary color */
      nprogress?: string;
      container: string;
      layout: string;
      inverted: string;
      'base-text': string;
    }

    interface ThemeSettingTokenBoxShadow {
      header: string;
      sider: string;
      tab: string;
    }

    interface ThemeSettingToken {
      colors: ThemeSettingTokenColor;
      boxShadow: ThemeSettingTokenBoxShadow;
    }

    type ThemeTokenColor = ThemePaletteColor & ThemeSettingTokenColor;

    /** Theme token CSS variables */
    type ThemeTokenCSSVars = {
      colors: ThemeTokenColor & { [key: string]: string };
      boxShadow: ThemeSettingTokenBoxShadow & { [key: string]: string };
    };
  }

  /** Global namespace */
  namespace Global {
    type VNode = import('vue').VNode;
    type RouteLocationNormalizedLoaded = import('vue-router').RouteLocationNormalizedLoaded;
    type RouteKey = import('@elegant-router/types').RouteKey;
    type RouteMap = import('@elegant-router/types').RouteMap;
    type RoutePath = import('@elegant-router/types').RoutePath;
    type LastLevelRouteKey = import('@elegant-router/types').LastLevelRouteKey;

    /** The router push options */
    type RouterPushOptions = {
      query?: Record<string, string>;
      params?: Record<string, string>;
      force?: boolean;
    };

    /** The global header props */
    interface HeaderProps {
      /** Whether to show the logo */
      showLogo?: boolean;
      /** Whether to show the menu toggler */
      showMenuToggler?: boolean;
      /** Whether to show the menu */
      showMenu?: boolean;
    }

    /** The global menu */
    type Menu = {
      /**
       * The menu key
       *
       * Equal to the route key
       */
      key: string;
      /** The menu label */
      label: string;
      /** The menu i18n key */
      i18nKey?: I18n.I18nKey | null;
      /** The route key */
      routeKey: RouteKey;
      /** The route path */
      routePath: RoutePath;
      /** The menu icon */
      icon?: () => VNode;
      /** The menu children */
      children?: Menu[];
    };

    type Breadcrumb = Omit<Menu, 'children'> & {
      options?: Breadcrumb[];
    };

    /** Tab route */
    type TabRoute = Pick<RouteLocationNormalizedLoaded, 'name' | 'path' | 'meta'> &
      Partial<Pick<RouteLocationNormalizedLoaded, 'fullPath' | 'query' | 'matched'>>;

    /** The global tab */
    type Tab = {
      /** The tab id */
      id: string;
      /** The tab label */
      label: string;
      /**
       * The new tab label
       *
       * If set, the tab label will be replaced by this value
       */
      newLabel?: string;
      /**
       * The old tab label
       *
       * when reset the tab label, the tab label will be replaced by this value
       */
      oldLabel?: string;
      /** The tab route key */
      routeKey: LastLevelRouteKey;
      /** The tab route path */
      routePath: RouteMap[LastLevelRouteKey];
      /** The tab route full path */
      fullPath: string;
      /** The tab fixed index */
      fixedIndex?: number | null;
      /**
       * Tab icon
       *
       * Iconify icon
       */
      icon?: string;
      /**
       * Tab local icon
       *
       * Local icon
       */
      localIcon?: string;
      /** I18n key */
      i18nKey?: I18n.I18nKey | null;
    };

    /** The global dropdown key */
    type DropdownKey = 'closeCurrent' | 'closeOther' | 'closeLeft' | 'closeRight' | 'closeAll' | 'pin' | 'unpin';
  }

  /**
   * I18n namespace
   *
   * Locales type
   */
  namespace I18n {
    type RouteKey = import('@elegant-router/types').RouteKey;

    type LangType = 'en-US' | 'zh-CN';

    type LangOption = {
      label: string;
      key: LangType;
    };

    type I18nRouteKey = Exclude<RouteKey, 'root' | 'not-found'>;

    type FormMsg = {
      required: string;
      invalid: string;
    };

    type Schema = {
    starter: {
      layout: string;
      layoutVertical: string;
      layoutHorizontal: string;
    };
    system: {
      title: string;
      updateTitle: string;
      updateContent: string;
      updateConfirm: string;
      updateCancel: string;
      footer: string;
      loginSubtitle: string;
    };
    common: {
      queryExample: string;
      standalone: string;
      autoAssign: string;
      hexPlaceholder: string;
      secondsNever: string;
      notFound: string;
      required: string;
      action: string;
      add: string;
      addSuccess: string;
      backToHome: string;
      batchDelete: string;
      cancel: string;
      close: string;
      check: string;
      selectAll: string;
      expandColumn: string;
      columnSetting: string;
      config: string;
      confirm: string;
      delete: string;
      deleteSuccess: string;
      confirmDelete: string;
      edit: string;
      warning: string;
      error: string;
      index: string;
      keywordSearch: string;
      logout: string;
      logoutConfirm: string;
      lookForward: string;
      modify: string;
      modifySuccess: string;
      noData: string;
      operate: string;
      pleaseCheckValue: string;
      refresh: string;
      reset: string;
      search: string;
      switch: string;
      tip: string;
      trigger: string;
      update: string;
      updateSuccess: string;
      userCenter: string;
      yesOrNo: {
        yes: string;
        no: string;
      };
      appearance: string;
      create: string;
      save: string;
      filter: string;
      export: string;
      import: string;
      enable: string;
      disable: string;
      enabled: string;
      url: string;
      notifications: string;
      exceptionTip: string;
      exception403: string;
      exception404: string;
      exception500: string;
      noNotifications: string;
      loadedRows: string;
      searchNoResult: string;
      disabled: string;
      experimental: string;
      actions: string;
      status: string;
      name: string;
      description: string;
      descriptions: string;
      type: string;
      value: string;
      priority: string;
      createdAt: string;
      updatedAt: string;
      total: string;
      loading: string;
      success: string;
      failed: string;
      deleteConfirm: string;
      createSuccess: string;
      noPermission: string;
      networkError: string;
      retry: string;
      copied: string;
      clipboardUnavailable: string;
      enterDomain: string;
      confirmRestore: string;
      language: string;
      yes: string;
      no: string;
    };
    request: {
      logout: string;
      logoutMsg: string;
      logoutWithModal: string;
      logoutWithModalMsg: string;
      refreshToken: string;
      tokenExpired: string;
    };
    theme: {
      themeDrawerTitle: string;
      tabs: {
        appearance: string;
        layout: string;
        general: string;
        preset: string;
      };
      appearance: {
        themeSchema: {
          title: string;
          light: string;
          dark: string;
          auto: string;
        };
        grayscale: string;
        colourWeakness: string;
        themeColor: {
          title: string;
          primary: string;
          info: string;
          success: string;
          warning: string;
          error: string;
          followPrimary: string;
        };
        themeRadius: {
          title: string;
        };
        elementSwitch: string;
        showBreadcrumb: string;
        useTagTabs: string;
        showFooter: string;
        fixedHeader: string;
        watermark: string;
        accessibility: string;
        recommendColor: string;
        recommendColorDesc: string;
        preset: {
          title: string;
          apply: string;
          applySuccess: string;
          default: {
            name: string;
            desc: string;
          };
          dark: {
            name: string;
            desc: string;
          };
          compact: {
            name: string;
            desc: string;
          };
          azir: {
            name: string;
            desc: string;
          };
        };
      };
      layout: {
        layoutMode: {
          title: string;
          vertical: string;
          "vertical-mix": string;
          "vertical-hybrid-header-first": string;
          horizontal: string;
          "top-hybrid-sidebar-first": string;
          "top-hybrid-header-first": string;
          vertical_detail: string;
          "vertical-mix_detail": string;
          "vertical-hybrid-header-first_detail": string;
          horizontal_detail: string;
          "top-hybrid-sidebar-first_detail": string;
          "top-hybrid-header-first_detail": string;
        };
        tab: {
          title: string;
          visible: string;
          cache: string;
          cacheTip: string;
          height: string;
          mode: {
            title: string;
            slider: string;
            chrome: string;
            button: string;
          };
          closeByMiddleClick: string;
          closeByMiddleClickTip: string;
        };
        header: {
          title: string;
          height: string;
          breadcrumb: {
            visible: string;
            showIcon: string;
          };
        };
        sider: {
          title: string;
          inverted: string;
          width: string;
          collapsedWidth: string;
          mixWidth: string;
          mixCollapsedWidth: string;
          mixChildMenuWidth: string;
          autoSelectFirstMenu: string;
          autoSelectFirstMenuTip: string;
        };
        footer: {
          title: string;
          visible: string;
          fixed: string;
          height: string;
          right: string;
        };
        content: {
          title: string;
          scrollMode: {
            title: string;
            tip: string;
            wrapper: string;
            content: string;
          };
          page: {
            animate: string;
            mode: {
              title: string;
              "fade-slide": string;
              fade: string;
              "fade-bottom": string;
              "fade-scale": string;
              "zoom-fade": string;
              "zoom-out": string;
              none: string;
            };
          };
          fixedHeaderAndTab: string;
        };
      };
      general: {
        title: string;
        watermark: {
          title: string;
          visible: string;
          text: string;
          enableUserName: string;
          enableTime: string;
          timeFormat: string;
        };
        multilingual: {
          title: string;
          visible: string;
        };
        globalSearch: {
          title: string;
          visible: string;
        };
      };
      configOperation: {
        copyConfig: string;
        copySuccessMsg: string;
        resetConfig: string;
        resetSuccessMsg: string;
      };
    };
    route: {
      "403": string;
      "404": string;
      "500": string;
      admin: string;
      admin_groups: string;
      admin_roles: string;
      admin_sessions: string;
      admin_tokens: string;
      admin_users: string;
      dashboard: string;
      dhcp: string;
      dhcp_leases: string;
      dhcp_options: string;
      dhcp_reservations: string;
      dhcp_scopes: string;
      dhcp_ha: string;
      dns: string;
      dns_allowed: string;
      dns_apps: string;
      dns_blocked: string;
      dns_cache: string;
      dns_forwarders: string;
      dns_security: string;
      dns_dnssec: string;
      "dns_zone-detail": string;
      dns_zones: string;
      "iframe-page": string;
      ipam: string;
      ipam_addresses: string;
      ipam_spaces: string;
      "ipam_retirement-archive": string;
      ipam_subnets: string;
      login: string;
      logs: string;
      logs_audit: string;
      logs_dhcp: string;
      logs_dns: string;
      settings: string;
      settings_system: string;
      settings_backup: string;
      "settings_config-versions": string;
      tools: string;
      tools_client: string;
    };
    page: {
      login: {
        common: {
          loginOrRegister: string;
          userNamePlaceholder: string;
          phonePlaceholder: string;
          codePlaceholder: string;
          passwordPlaceholder: string;
          confirmPasswordPlaceholder: string;
          codeLogin: string;
          confirm: string;
          back: string;
          validateSuccess: string;
          loginSuccess: string;
          welcomeBack: string;
        };
        pwdLogin: {
          title: string;
          rememberMe: string;
          forgetPassword: string;
          register: string;
          otherAccountLogin: string;
          otherLoginMode: string;
          superAdmin: string;
          admin: string;
          user: string;
        };
        codeLogin: {
          title: string;
          getCode: string;
          reGetCode: string;
          sendCodeSuccess: string;
          imageCodePlaceholder: string;
        };
        register: {
          title: string;
          agreement: string;
          protocol: string;
          policy: string;
        };
        resetPwd: {
          title: string;
        };
        bindWeChat: {
          title: string;
        };
      };
      home: {
        branchDesc: string;
        greeting: string;
        weatherDesc: string;
        projectCount: string;
        todo: string;
        message: string;
        downloadCount: string;
        registerCount: string;
        schedule: string;
        study: string;
        work: string;
        rest: string;
        entertainment: string;
        visitCount: string;
        turnover: string;
        dealCount: string;
        projectNews: {
          title: string;
          moreNews: string;
          desc1: string;
          desc2: string;
          desc3: string;
          desc4: string;
          desc5: string;
        };
        creativity: string;
      };
    };
    form: {
      required: string;
      userName: {
        required: string;
        invalid: string;
      };
      phone: {
        required: string;
        invalid: string;
      };
      pwd: {
        required: string;
        invalid: string;
      };
      confirmPwd: {
        required: string;
        invalid: string;
      };
      code: {
        required: string;
        invalid: string;
      };
      email: {
        required: string;
        invalid: string;
      };
    };
    dropdown: {
      closeCurrent: string;
      closeOther: string;
      closeLeft: string;
      closeRight: string;
      closeAll: string;
      pin: string;
      unpin: string;
    };
    icon: {
      themeConfig: string;
      themeSchema: string;
      lang: string;
      fullscreen: string;
      fullscreenExit: string;
      reload: string;
      collapse: string;
      expand: string;
      pin: string;
      unpin: string;
    };
    datatable: {
      itemCount: string;
      fixed: {
        left: string;
        right: string;
        unFixed: string;
      };
    };
    perm: {
      resource: {
        dns: string;
        dhcp: string;
        ipam: string;
        user: string;
        role: string;
        group: string;
        settings: string;
        audit: string;
        backup: string;
        token: string;
      };
      action: {
        read: string;
        write: string;
        delete: string;
      };
    };
    sessions: {
      title: string;
      sessionId: string;
      user: string;
      ip: string;
      createdAt: string;
      expiresAt: string;
      revoke: string;
      revokeOthers: string;
      revokeConfirm: string;
      revokeOthersConfirm: string;
      revokeSuccess: string;
      revokePartial: string;
      revokeFailed: string;
    };
    auth: {
      login: string;
      logout: string;
      username: string;
      password: string;
      totpCode: string;
      loginFailed: string;
      loginSuccess: string;
      sessionExpired: string;
      changePassword: string;
      oldPassword: string;
      newPassword: string;
      confirmPassword: string;
      usernameRequired: string;
      passwordRequired: string;
      passwordMinLength: string;
    };
    dashboard: {
      title: string;
      hour: string;
      day: string;
      week: string;
      month: string;
      year: string;
      dnsQueryTrend: string;
      dnsStats: string;
      rcodeDistribution: string;
      dnsQueriesToday: string;
      cacheHitRate: string;
      activeLeases: string;
      ipamUsage: string;
      rank: string;
      count: string;
      trend: string;
      authorVersion: string;
      sinceLastWeek: string;
      rcodeNoError: string;
      rankInfo: string;
      recentEvents: string;
      systemStatus: string;
      queryChart: string;
      topStats: string;
      rangeHour: string;
      rangeDay: string;
      rangeWeek: string;
      rangeMonth: string;
      rangeYear: string;
      topClients: string;
      topDomains: string;
      topBlocked: string;
      rcodes: string;
      longTermStats: string;
      statsTotal: string;
      statsNoError: string;
      statsNxDomain: string;
      statsServFail: string;
      statsRefused: string;
      statsBlocked: string;
      statsCached: string;
      statsClients: string;
      statsAvgLatency: string;
      rcodesOther: string;
    };
    dns: {
      zones: {
        title: string;
        zoneName: string;
        zoneNameInvalid: string;
        zoneType: string;
        tabAuthoritative: string;
        tabAllowed: string;
        tabBlocked: string;
        typeAllowed: string;
        typeBlocked: string;
        typeValues: {
          primary: string;
          secondary: string;
          stub: string;
          forward: string;
          catalog: string;
        };
        specialHint: string;
        specialHintText: string;
        aclTitle: string;
        aclAllowQuery: string;
        aclAllowTransfer: string;
        aclAllowUpdate: string;
        aclNotify: string;
        aclHint: string;
        queryAccess: string;
        queryAccessDefault: string;
        queryAccessAllow: string;
        queryAccessDeny: string;
        queryAccessPrivate: string;
        tabRecords: string;
        tabOptions: string;
        tabPermissions: string;
        tabHistory: string;
        tabDnssec: string;
        permAdd: string;
        permPrincipalType: string;
        permPrincipalUser: string;
        permPrincipalGroup: string;
        permPrincipalId: string;
        permView: string;
        permModify: string;
        permDelete: string;
        permHint: string;
        catalogTitle: string;
        catalogJoin: string;
        catalogNone: string;
        unsavedChanges: string;
        catalogHint: string;
        catalogMembers: string;
        catalogMembersEmpty: string;
        historyEmpty: string;
        records: string;
        dnssec: string;
        createZone: string;
        editZone: string;
        primaryNs: string;
        adminEmail: string;
        serial: string;
        refresh: string;
        retry: string;
        expire: string;
        minimum: string;
        ttl: string;
        importZone: string;
        importPreviewHint: string;
        importContentPlaceholder: string;
        importPreview: string;
        importApply: string;
        importPreviewValid: string;
        importPreviewConflicts: string;
        importRecordCount: string;
        importConflictRow: string;
        importSuccess: string;
        exportZone: string;
        syncZone: string;
        enableDnssec: string;
        disableDnssec: string;
        rotateKeys: string;
        dnssecGenerateKey: string;
        dnssecPromote: string;
        dnssecShowDs: string;
        dnssecDsTitle: string;
        dnssecNsec3Title: string;
        dnssecNsec3Iterations: string;
        dnssecNsec3Salt: string;
        dnssecNsec3Optout: string;
        dnssecNsec3Hint: string;
        dnssecExperimentalHint: string;
        clone: string;
        cloneTitle: string;
        newName: string;
        newNameRequired: string;
        convert: string;
        convertTitle: string;
        convertTarget: string;
        convertHint: string;
        batchDelete: string;
        batchDeleteConfirm: string;
        batchDeleteDone: string;
        selectedCount: string;
      };
      records: {
        title: string;
        recordName: string;
        recordType: string;
        recordValue: string;
        createRecord: string;
        editRecord: string;
        batchCreate: string;
        batchDelete: string;
      };
      forwarders: {
        title: string;
        protocol: string;
        address: string;
        conditionalForwarders: string;
        domain: string;
        createForwarder: string;
        createConditional: string;
      };
      apps: {
        title: string;
        notAvailable: string;
        colId: string;
        colName: string;
        colVersion: string;
        colStatus: string;
        statusOn: string;
        statusOff: string;
        notImplemented: string;
        hint: string;
      };
      dnssec: {
        title: string;
        experimentalHint: string;
        zone: string;
        selectZone: string;
        status: string;
        keyCount: string;
        keyLifecycle: string;
        keyTag: string;
        keyId: string;
        digestType: string;
        digest: string;
        algorithm: string;
        keyType: string;
        createdAt: string;
        enable: string;
        disable: string;
        rotate: string;
        generateKey: string;
        promote: string;
        deleteKeyConfirm: string;
        noKeys: string;
        dsTitle: string;
        noDs: string;
        dsHint: string;
        nsec3Title: string;
        iterations: string;
        salt: string;
        optout: string;
        nsec3Hint: string;
      };
      security: {
        title: string;
        allowedTitle: string;
        blockedTitle: string;
        flush: string;
        flushConfirm: string;
        flushSuccess: string;
        importRules: string;
        exportRules: string;
        importSuccess: string;
        blockLists: string;
        allowRules: string;
        clientPolicies: string;
        pattern: string;
        action: string;
        matchType: string;
        responseType: string;
        matchValues: {
          exact: string;
          suffix: string;
          wildcard: string;
          regex: string;
        };
        responseValues: {
          nxdomain: string;
          nodata: string;
          refused: string;
          drop: string;
          custom_ip: string;
        };
        listTypes: {
          custom: string;
          external: string;
        };
        actionValues: {
          allow: string;
          block: string;
        };
        applyLists: string;
        rules: string;
        sourceIp: string;
        sourceCidr: string;
        createBlockList: string;
        addBlockRule: string;
        addAllowRule: string;
        createPolicy: string;
        temporaryDisable: string;
        disableNow: string;
        disableMinutes: string;
        blockingDisabledUntil: string;
        resumeBlocking: string;
        disableSuccess: string;
        resumeSuccess: string;
        lastFetch: string;
        neverFetched: string;
        fetchFailed: string;
        refresh: string;
        refreshSuccess: string;
      };
      cache: {
        title: string;
        entries: string;
        maxEntries: string;
        hitRate: string;
        missRate: string;
        flushCache: string;
        flushEntry: string;
        flushConfirm: string;
        entryList: string;
        filterQname: string;
        colName: string;
        colType: string;
        colTTL: string;
        colHits: string;
        colExpires: string;
      };
      client: {
        upstreamHint: string;
        localResult: string;
        externalResult: string;
        source: string;
        unknownSource: string;
        title: string;
        importToZone: string;
        selectZone: string;
        queryName: string;
        queryType: string;
        upstream: string;
        execute: string;
        response: string;
        queryTime: string;
        server: string;
      };
    };
    dhcp: {
      scopes: {
        title: string;
        subnet: string;
        startIp: string;
        endIp: string;
        leaseTime: string;
        activeLeases: string;
        totalAddresses: string;
        createScope: string;
        editScope: string;
        usage: string;
      };
      leases: {
        title: string;
        ip: string;
        mac: string;
        hostname: string;
        scope: string;
        startTime: string;
        endTime: string;
        release: string;
        clientId: string;
        statusValues: {
          active: string;
          offered: string;
          expired: string;
          released: string;
          conflict: string;
        };
      };
      reservations: {
        title: string;
        createReservation: string;
        editReservation: string;
      };
      options: {
        title: string;
        code: string;
        optionName: string;
        optionValue: string;
        scopeRequired: string;
        scope: string;
        createOption: string;
        editOption: string;
      };
    };
    ipam: {
      archive: {
        title: string;
        kind: string;
        resourceId: string;
        retiredAt: string;
        retiredBy: string;
        reason: string;
        allocatedChildren: string;
        historyChildren: string;
        kindValues: {
          space: string;
          subnet: string;
        };
        localOnlyHint: string;
        empty: string;
        restore: string;
        restoreConfirm: string;
        restoreSuccess: string;
        purge: string;
        purgeConfirm: string;
        purgeSuccess: string;
      };
      lifecycleActive: string;
      lifecycleRetired: string;
      lifecycleAll: string;
      spaces: {
        title: string;
        createSpace: string;
        editSpace: string;
        retire: string;
        retireConfirm: string;
        retireSuccess: string;
      };
      subnets: {
        title: string;
        cidr: string;
        vlanId: string;
        location: string;
        createSubnet: string;
        editSubnet: string;
        createDhcpScope: string;
        generateReverseZone: string;
        importAddresses: string;
        retire: string;
        retireConfirm: string;
        retireSuccess: string;
      };
      reverseZone: {
        title: string;
        cidr: string;
        candidates: string;
        candidatesHint: string;
        mostSpecific: string;
        ttl: string;
        create: string;
        created: string;
        willCreate: string;
        noPermission: string;
        loadFailed: string;
      };
      addresses: {
        title: string;
        ip: string;
        subnet: string;
        hostname: string;
        mac: string;
        owner: string;
        device: string;
        location: string;
        allocate: string;
        release: string;
        viewDetail: string;
        statusUsed: string;
        statusFree: string;
        statusReserved: string;
        statusConflict: string;
      };
      detail: {
        title: string;
        permissionLimited: string;
        nothingToShow: string;
        conflicts: string;
        space: string;
        lastSeen: string;
        dnsRecords: string;
        dnsRecordsEmpty: string;
        recordName: string;
        recordType: string;
        recordValue: string;
        ttl: string;
        author: string;
        authorLocal: string;
        authorOperator: string;
        scopes: string;
        scopesTruncated: string;
        scopesEmpty: string;
        scopesUnknown: string;
        inSubnet: string;
        inPool: string;
        isGateway: string;
        range: string;
        router: string;
        relation: string;
        leases: string;
        leasesEmpty: string;
        leaseEnd: string;
        reservations: string;
        reservationsEmpty: string;
        history: string;
        historyEmpty: string;
        changedBy: string;
        reason: string;
        source: string;
      };
      import: {
        title: string;
        chooseFile: string;
        pastePlaceholder: string;
        preview: string;
        previewTitle: string;
        confirm: string;
        appliedTitle: string;
        back: string;
        close: string;
        total: string;
        creates: string;
        updates: string;
        unchanged: string;
        errors: string;
        rejected: string;
        rejectedHint: string;
        changes: string;
        truncated: string;
        line: string;
        ip: string;
        action: string;
        statusChange: string;
        changedFields: string;
        actionCreate: string;
        actionUpdate: string;
        actionUnchanged: string;
      };
      pool: {
        title: string;
        planFailed: string;
        stale: string;
        staleHint: string;
        stepPlan: string;
        stepConfirm: string;
        conflicts: string;
        warnings: string;
        gatewayConventionWarning: string;
        scopeOverlapConflict: string;
        scopeDisabledOverlapWarning: string;
        scopeSameSubnetWarning: string;
        range: string;
        router: string;
        routerSource: string;
        routerFromGateway: string;
        routerFromConvention: string;
        routerFromNothing: string;
        leaseTime: string;
        fingerprint: string;
        excluded: string;
        excludedEmpty: string;
        excludedHint: string;
        mask: string;
        confirmHint: string;
        back: string;
        replan: string;
        next: string;
        create: string;
        noDhcpPermission: string;
        noDhcpPermissionHint: string;
        created: string;
      };
    };
    admin: {
      sessions: {
        title: string;
        ip: string;
        userAgent: string;
        expiresAt: string;
        revoke: string;
        revoked: string;
      };
      users: {
        title: string;
        username: string;
        email: string;
        displayName: string;
        enabled: string;
        totpEnabled: string;
        roles: string;
        groups: string;
        createUser: string;
        editUser: string;
        assignRoles: string;
        assignGroups: string;
      };
      roles: {
        title: string;
        permissions: string;
        isSystem: string;
        createRole: string;
        editRole: string;
        assignPermissions: string;
        builtin: {
          admin: string;
          operator: string;
          viewer: string;
        };
        builtinDesc: {
          admin: string;
          operator: string;
          viewer: string;
        };
      };
      groups: {
        title: string;
        createGroup: string;
        editGroup: string;
        assignRoles: string;
      };
      tokens: {
        title: string;
        tokenName: string;
        expiresAt: string;
        lastUsedAt: string;
        createToken: string;
        tokenCreated: string;
        tokenWarning: string;
        copyToken: string;
      };
    };
    logs: {
      audit: {
        title: string;
        user: string;
        action: string;
        resource: string;
        resourceId: string;
        details: string;
        ip: string;
        actionValues: {
          create: string;
          update: string;
          delete: string;
          login: string;
          logout: string;
          publish: string;
          rollback: string;
          retire: string;
          get: string;
          assign_role: string;
          set_permissions: string;
          dns_dynamic_update: string;
          ddns_record_create: string;
          ddns_record_delete: string;
          dhcp_lease_bind: string;
          dhcp_lease_release: string;
          dhcp_lease_decline: string;
          dhcp_lease_expire: string;
          dhcp_ha_degrade: string;
          dhcp_ha_takeover: string;
          dhcp_ha_fence: string;
          dhcp_ha_rejoin: string;
        };
      };
      dns: {
        title: string;
        queryName: string;
        queryType: string;
        clientIp: string;
        responseCode: string;
        responseTime: string;
        cached: string;
        blocked: string;
        upstream: string;
        exportCsv: string;
        liveOn: string;
        liveOff: string;
      };
      dhcp: {
        title: string;
        eventType: string;
        clientMac: string;
        clientIp: string;
        hostname: string;
        scope: string;
        message: string;
        eventValues: {
          discover: string;
          offer: string;
          request: string;
          ack: string;
          nak: string;
          decline: string;
          release: string;
          inform: string;
          conflict: string;
        };
      };
    };
    settings: {
      title: string;
      key: string;
      value: string;
      updateSuccess: string;
      items: {
        server_name: {
          label: string;
          desc: string;
        };
        server_language: {
          label: string;
          desc: string;
        };
        server_dark_mode: {
          label: string;
          desc: string;
        };
        dns_default_ttl: {
          label: string;
          desc: string;
        };
        dns_recursion: {
          label: string;
          desc: string;
        };
        dns_blocking_enabled: {
          label: string;
          desc: string;
        };
        dns_rate_limit_qps: {
          label: string;
          desc: string;
        };
        dns_blocklist_refresh_hours: {
          label: string;
          desc: string;
        };
        dns_ecs_mode: {
          label: string;
          desc: string;
        };
        dns_ecs_ipv4_prefix_length: {
          label: string;
          desc: string;
        };
        dns_ecs_ipv6_prefix_length: {
          label: string;
          desc: string;
        };
        dns_cache_serve_stale: {
          label: string;
          desc: string;
        };
        dns_cache_stale_ttl: {
          label: string;
          desc: string;
        };
        dns_cache_prefetch: {
          label: string;
          desc: string;
        };
        dns_cache_min_ttl: {
          label: string;
          desc: string;
        };
        dns_cache_max_ttl: {
          label: string;
          desc: string;
        };
        dhcp_lease_time: {
          label: string;
          desc: string;
        };
        ipam_ping_check: {
          label: string;
          desc: string;
        };
        ipam_auto_scan: {
          label: string;
          desc: string;
        };
        security_rebinding: {
          label: string;
          desc: string;
        };
        log_retention_days: {
          label: string;
          desc: string;
        };
        backup_auto_enabled: {
          label: string;
          desc: string;
        };
        backup_auto_schedule: {
          label: string;
          desc: string;
        };
        backup_retention: {
          label: string;
          desc: string;
        };
      };
      backup: {
        title: string;
        backupName: string;
        backupSize: string;
        backupType: string;
        backupStatus: string;
        createBackup: string;
        downloadBackup: string;
        restoreBackup: string;
        restoreConfirm: string;
        deleteBackup: string;
        statusCompleted: string;
        statusFailed: string;
        statusRunning: string;
        typeFull: string;
      };
      configVersions: {
        title: string;
        resourceTypesTitle: string;
        resourceTypesEmpty: string;
        resourceTypes: {
          dns_zone: string;
          dhcp_scope: string;
          ipam_subnet: string;
          dns_records: string;
        };
        revisionsTitle: string;
        noteValues: {
          created: string;
          rollback: string;
          updated: string;
          published: string;
          imported: string;
          restored: string;
        };
        filterResourceType: string;
        filterResourceId: string;
        resourceIdPlaceholder: string;
        emptyRevisions: string;
        revision: string;
        resourceType: string;
        resourceId: string;
        status: string;
        statusLabels: {
          persisted: string;
          staged: string;
          applied: string;
          failed: string;
        };
        actor: string;
        note: string;
        createdAt: string;
        appliedAt: string;
        appliedGeneration: string;
        rollback: string;
        rollbackTitle: string;
        rollbackTarget: string;
        rollbackExpected: string;
        rollbackExpectedHint: string;
        rollbackNote: string;
        rollbackConfirm: string;
        rollbackSuccess: string;
        conflictBody: string;
        diffTitle: string;
        diffHint: string;
        diffFrom: string;
        diffTo: string;
        computeDiff: string;
        noChanges: string;
        fieldChanges: string;
        noFieldChanges: string;
        field: string;
        oldValue: string;
        newValue: string;
        recordChanges: string;
        recordChangesHint: string;
        noRecordChanges: string;
        recordDiffKind: {
          added: string;
          removed: string;
          changed: string;
        };
        recordDiffSummary: string;
        recordName: string;
        recordType: string;
        recordValue: string;
        changedFields: string;
        publishTitle: string;
        publishHint: string;
        publishApiOnly: string;
        contentJson: string;
        dryRun: string;
        dryRunResult: string;
        dryRunBaseRevision: string;
        dryRunReplayed: string;
        writeRequired: string;
        invalidJson: string;
        queueTitle: string;
        queuePending: string;
        queueFailed: string;
        queueAttempts: string;
        queueLastError: string;
        queueEmpty: string;
        retryFailed: string;
        retry: string;
        retryResult: string;
      };
    };
    haStatus: {
      subtitle: string;
      stateValues: {
        primary: string;
        standby: string;
        "primary-degraded": string;
        paused: string;
        fenced: string;
        unknown: string;
      };
      roleValues: {
        primary: string;
        standby: string;
        monitor: string;
      };
      standalone: string;
      autoOn: string;
      autoOff: string;
      state: string;
      redundant: string;
      promising: string;
      nodeId: string;
      configuredRole: string;
      roleInForce: string;
      sequence: string;
      acknowledged: string;
      peerApplied: string;
      peerFacts: string;
      peerLastSeen: string;
      source: string;
      readOnly: string;
      replAlertTitle: string;
      replAlertHint: string;
      replHealthy: string;
      replLastSync: string;
      replSource: string;
      replUnknown: string;
      replLoadError: string;
      replExpected: string;
      replApplied: string;
      replBehindTitle: string;
      replBehindHint: string;
    };
    nodeMode: {
      single: string;
      primary: string;
      standby: string;
      degraded: string;
    };
  };;

    type GetI18nKey<T extends Record<string, unknown>, K extends keyof T = keyof T> = K extends string
      ? T[K] extends Record<string, unknown>
        ? `${K}.${GetI18nKey<T[K]>}`
        : K
      : never;

    type I18nKey = GetI18nKey<Schema>;

    type TranslateOptions<Locales extends string> = import('vue-i18n').TranslateOptions<Locales>;

    interface $T {
      (key: I18nKey): string;
      (key: I18nKey, plural: number, options?: TranslateOptions<LangType>): string;
      (key: I18nKey, defaultMsg: string, options?: TranslateOptions<I18nKey>): string;
      (key: I18nKey, list: unknown[], options?: TranslateOptions<I18nKey>): string;
      (key: I18nKey, list: unknown[], plural: number): string;
      (key: I18nKey, list: unknown[], defaultMsg: string): string;
      (key: I18nKey, named: Record<string, unknown>, options?: TranslateOptions<LangType>): string;
      (key: I18nKey, named: Record<string, unknown>, plural: number): string;
      (key: I18nKey, named: Record<string, unknown>, defaultMsg: string): string;
    }
  }

  /** Service namespace */
  namespace Service {
    /** Other baseURL key */
    type OtherBaseURLKey = 'demo';

    interface ServiceConfigItem {
      /** The backend service base url */
      baseURL: string;
      /** The proxy pattern of the backend service base url */
      proxyPattern: string;
    }

    interface OtherServiceConfigItem extends ServiceConfigItem {
      key: OtherBaseURLKey;
    }

    /** The backend service config */
    interface ServiceConfig extends ServiceConfigItem {
      /** Other backend service config */
      other: OtherServiceConfigItem[];
    }

    interface SimpleServiceConfig extends Pick<ServiceConfigItem, 'baseURL'> {
      other: Record<OtherBaseURLKey, string>;
    }

    /** The backend service response data */
    type Response<T = unknown> = {
      /** The backend service response code */
      code: string;
      /** The backend service response message */
      msg: string;
      /** The backend service response data */
      data: T;
    };

    /** The demo backend service response data */
    type DemoResponse<T = unknown> = {
      /** The backend service response code */
      status: string;
      /** The backend service response message */
      message: string;
      /** The backend service response data */
      result: T;
    };
  }
}
