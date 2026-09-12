import{E as e,g as t,k as n}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{Bn as r,Br as i,Fn as a,Pn as o,gn as s,yn as c,zr as l}from"./router-L4VEvoUg.js";import{a as u}from"./index-DhGv5Rfg.js";var d=l(`statistic`,[i(`label`,`
 font-weight: var(--n-label-font-weight);
 transition: .3s color var(--n-bezier);
 font-size: var(--n-label-font-size);
 color: var(--n-label-text-color);
 `),l(`statistic-value`,`
 margin-top: 4px;
 font-weight: var(--n-value-font-weight);
 `,[i(`prefix`,`
 margin: 0 4px 0 0;
 font-size: var(--n-value-font-size);
 transition: .3s color var(--n-bezier);
 color: var(--n-value-prefix-text-color);
 `,[l(`icon`,{verticalAlign:`-0.125em`})]),i(`content`,`
 font-size: var(--n-value-font-size);
 transition: .3s color var(--n-bezier);
 color: var(--n-value-text-color);
 `),i(`suffix`,`
 margin: 0 0 0 4px;
 font-size: var(--n-value-font-size);
 transition: .3s color var(--n-bezier);
 color: var(--n-value-suffix-text-color);
 `,[l(`icon`,{verticalAlign:`-0.125em`})])])]),f=e({name:`Statistic`,props:Object.assign(Object.assign({},s.props),{tabularNums:Boolean,label:String,value:[String,Number]}),slots:Object,setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:r,mergedRtlRef:i}=a(e),l=s(`Statistic`,`-statistic`,d,u,e,n),f=c(`Statistic`,i,n),p=t(()=>{let{self:{labelFontWeight:e,valueFontSize:t,valueFontWeight:n,valuePrefixTextColor:r,labelTextColor:i,valueSuffixTextColor:a,valueTextColor:o,labelFontSize:s},common:{cubicBezierEaseInOut:c}}=l.value;return{"--n-bezier":c,"--n-label-font-size":s,"--n-label-font-weight":e,"--n-label-text-color":i,"--n-value-font-weight":n,"--n-value-font-size":t,"--n-value-prefix-text-color":r,"--n-value-suffix-text-color":a,"--n-value-text-color":o}}),m=r?o(`statistic`,void 0,p,e):void 0;return{rtlEnabled:f,mergedClsPrefix:n,cssVars:r?void 0:p,themeClass:m?.themeClass,onRender:m?.onRender}},render(){var e;let{mergedClsPrefix:t,$slots:{default:i,label:a,prefix:o,suffix:s}}=this;return(e=this.onRender)==null||e.call(this),n(`div`,{class:[`${t}-statistic`,this.themeClass,this.rtlEnabled&&`${t}-statistic--rtl`],style:this.cssVars},r(a,e=>n(`div`,{class:`${t}-statistic__label`},this.label||e)),n(`div`,{class:`${t}-statistic-value`,style:{fontVariantNumeric:this.tabularNums?`tabular-nums`:``}},r(o,e=>e&&n(`span`,{class:`${t}-statistic-value__prefix`},e)),this.value===void 0?r(i,e=>e&&n(`span`,{class:`${t}-statistic-value__content`},e)):n(`span`,{class:`${t}-statistic-value__content`},this.value),r(s,e=>e&&n(`span`,{class:`${t}-statistic-value__suffix`},e))))}});export{f as t};