import{E as e,g as t,k as n}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{Ar as r,Dn as i,Fr as a,Mr as o,Nr as s,On as c,Or as l,Pr as u,hr as d,ir as f,jr as p,kr as m,un as h,zn as g}from"./router-mKxr9Z2z.js";import{l as _}from"./DataTable-DGCEeHcg.js";import{l as v}from"./index-Dcl5rJ-i.js";function y(e,t=`default`,n=[]){let{children:r}=e;if(typeof r==`object`&&r&&!Array.isArray(r)){let e=r[t];if(typeof e==`function`)return e()}return n}var b=l([m(`descriptions`,{fontSize:`var(--n-font-size)`},[m(`descriptions-separator`,`
 display: inline-block;
 margin: 0 8px 0 2px;
 `),m(`descriptions-table-wrapper`,[m(`descriptions-table`,[m(`descriptions-table-row`,[m(`descriptions-table-header`,{padding:`var(--n-th-padding)`}),m(`descriptions-table-content`,{padding:`var(--n-td-padding)`})])])]),o(`bordered`,[m(`descriptions-table-wrapper`,[m(`descriptions-table`,[m(`descriptions-table-row`,[l(`&:last-child`,[m(`descriptions-table-content`,{paddingBottom:0})])])])])]),p(`left-label-placement`,[m(`descriptions-table-content`,[l(`> *`,{verticalAlign:`top`})])]),p(`left-label-align`,[l(`th`,{textAlign:`left`})]),p(`center-label-align`,[l(`th`,{textAlign:`center`})]),p(`right-label-align`,[l(`th`,{textAlign:`right`})]),p(`bordered`,[m(`descriptions-table-wrapper`,`
 border-radius: var(--n-border-radius);
 overflow: hidden;
 background: var(--n-merged-td-color);
 border: 1px solid var(--n-merged-border-color);
 `,[m(`descriptions-table`,[m(`descriptions-table-row`,[l(`&:not(:last-child)`,[m(`descriptions-table-content`,{borderBottom:`1px solid var(--n-merged-border-color)`}),m(`descriptions-table-header`,{borderBottom:`1px solid var(--n-merged-border-color)`})]),m(`descriptions-table-header`,`
 font-weight: 400;
 background-clip: padding-box;
 background-color: var(--n-merged-th-color);
 `,[l(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})]),m(`descriptions-table-content`,[l(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})])])])])]),m(`descriptions-header`,`
 font-weight: var(--n-th-font-weight);
 font-size: 18px;
 transition: color .3s var(--n-bezier);
 line-height: var(--n-line-height);
 margin-bottom: 16px;
 color: var(--n-title-text-color);
 `),m(`descriptions-table-wrapper`,`
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[m(`descriptions-table`,`
 width: 100%;
 border-collapse: separate;
 border-spacing: 0;
 box-sizing: border-box;
 `,[m(`descriptions-table-row`,`
 box-sizing: border-box;
 transition: border-color .3s var(--n-bezier);
 `,[m(`descriptions-table-header`,`
 font-weight: var(--n-th-font-weight);
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-th-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `),m(`descriptions-table-content`,`
 vertical-align: top;
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-td-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[r(`content`,`
 transition: color .3s var(--n-bezier);
 display: inline-block;
 color: var(--n-td-text-color);
 `)]),r(`label`,`
 font-weight: var(--n-th-font-weight);
 transition: color .3s var(--n-bezier);
 display: inline-block;
 margin-right: 14px;
 color: var(--n-th-text-color);
 `)])])])]),m(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color);
 --n-merged-td-color: var(--n-td-color);
 --n-merged-border-color: var(--n-border-color);
 `),u(m(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-modal);
 --n-merged-td-color: var(--n-td-color-modal);
 --n-merged-border-color: var(--n-border-color-modal);
 `)),a(m(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-popover);
 --n-merged-td-color: var(--n-td-color-popover);
 --n-merged-border-color: var(--n-border-color-popover);
 `))]),x=`DESCRIPTION_ITEM_FLAG`;function S(e){return typeof e==`object`&&e&&!Array.isArray(e)?e.type&&e.type.DESCRIPTION_ITEM_FLAG:!1}var C=e({name:`Descriptions`,props:Object.assign(Object.assign({},h.props),{title:String,column:{type:Number,default:3},columns:Number,labelPlacement:{type:String,default:`top`},labelAlign:{type:String,default:`left`},separator:{type:String,default:`:`},size:String,bordered:Boolean,labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]}),slots:Object,setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:r,mergedComponentPropsRef:a}=c(e),o=t(()=>e.size||a?.value?.Descriptions?.size||`medium`),l=h(`Descriptions`,`-descriptions`,b,v,e,n),u=t(()=>{let{bordered:t}=e,n=o.value,{common:{cubicBezierEaseInOut:r},self:{titleTextColor:i,thColor:a,thColorModal:c,thColorPopover:u,thTextColor:d,thFontWeight:f,tdTextColor:p,tdColor:m,tdColorModal:h,tdColorPopover:g,borderColor:_,borderColorModal:v,borderColorPopover:y,borderRadius:b,lineHeight:x,[s(`fontSize`,n)]:S,[s(t?`thPaddingBordered`:`thPadding`,n)]:C,[s(t?`tdPaddingBordered`:`tdPadding`,n)]:w}}=l.value;return{"--n-title-text-color":i,"--n-th-padding":C,"--n-td-padding":w,"--n-font-size":S,"--n-bezier":r,"--n-th-font-weight":f,"--n-line-height":x,"--n-th-text-color":d,"--n-td-text-color":p,"--n-th-color":a,"--n-th-color-modal":c,"--n-th-color-popover":u,"--n-td-color":m,"--n-td-color-modal":h,"--n-td-color-popover":g,"--n-border-radius":b,"--n-border-color":_,"--n-border-color-modal":v,"--n-border-color-popover":y}}),d=r?i(`descriptions`,t(()=>{let t=``,{bordered:n}=e;return n&&(t+=`a`),t+=o.value[0],t}),u,e):void 0;return{mergedClsPrefix:n,cssVars:r?void 0:u,themeClass:d?.themeClass,onRender:d?.onRender,compitableColumn:f(e,[`columns`,`column`]),inlineThemeDisabled:r,mergedSize:o}},render(){let e=this.$slots.default,t=e?g(e()):[];t.length;let{contentClass:r,labelClass:i,compitableColumn:a,labelPlacement:o,labelAlign:s,mergedSize:c,bordered:l,title:u,cssVars:f,mergedClsPrefix:p,separator:m,onRender:h}=this;h?.();let v=t.filter(e=>S(e)),b=v.reduce((e,t,s)=>{let c=t.props||{},u=v.length-1===s,d=[`label`in c?c.label:y(t,`label`)],f=[y(t)],h=c.span||1,g=e.span;e.span+=h;let _=c.labelStyle||c[`label-style`]||this.labelStyle,b=c.contentStyle||c[`content-style`]||this.contentStyle;if(o===`left`)l?e.row.push(n(`th`,{class:[`${p}-descriptions-table-header`,i],colspan:1,style:_},d),n(`td`,{class:[`${p}-descriptions-table-content`,r],colspan:u?(a-g)*2+1:h*2-1,style:b},f)):e.row.push(n(`td`,{class:`${p}-descriptions-table-content`,colspan:u?(a-g)*2:h*2},n(`span`,{class:[`${p}-descriptions-table-content__label`,i],style:_},[...d,m&&n(`span`,{class:`${p}-descriptions-separator`},m)]),n(`span`,{class:[`${p}-descriptions-table-content__content`,r],style:b},f)));else{let t=u?(a-g)*2:h*2;e.row.push(n(`th`,{class:[`${p}-descriptions-table-header`,i],colspan:t,style:_},d)),e.secondRow.push(n(`td`,{class:[`${p}-descriptions-table-content`,r],colspan:t,style:b},f))}return(e.span>=a||u)&&(e.span=0,e.row.length&&(e.rows.push(e.row),e.row=[]),o!==`left`&&e.secondRow.length&&(e.rows.push(e.secondRow),e.secondRow=[])),e},{span:0,row:[],secondRow:[],rows:[]}).rows.map(e=>n(`tr`,{class:`${p}-descriptions-table-row`},e));return n(`div`,{style:f,class:[`${p}-descriptions`,this.themeClass,`${p}-descriptions--${o}-label-placement`,`${p}-descriptions--${s}-label-align`,`${p}-descriptions--${c}-size`,l&&`${p}-descriptions--bordered`]},u||this.$slots.header?n(`div`,{class:`${p}-descriptions-header`},u||_(this,`header`)):null,n(`div`,{class:`${p}-descriptions-table-wrapper`},n(`table`,{class:`${p}-descriptions-table`},n(`tbody`,null,o===`top`&&n(`tr`,{class:`${p}-descriptions-table-row`,style:{visibility:`collapse`}},d(a*2,n(`td`,null))),b))))}}),w={label:String,span:{type:Number,default:1},labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]},T=e({name:`DescriptionsItem`,[x]:!0,props:w,slots:Object,render(){return null}});export{C as n,T as t};