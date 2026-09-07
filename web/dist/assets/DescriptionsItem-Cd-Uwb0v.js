import{O as e,j as t,v as n}from"./echarts-eUEtiXc8.js";import{D as r}from"./vue-core-CcEAoDdX.js";import{$ as i,G as a,J as o,K as s,N as c,P as l,Q as u,X as d,Y as f,Z as p,i as m}from"./light-BKCENzy2.js";import{t as h}from"./use-compitable-DSN36vHZ.js";import{_ as g}from"./DataTable-Dk4L6pRr.js";import{h as _,st as v}from"./index-XXeKGbje.js";function y(e,t=`default`,n=[]){let{children:r}=e;if(typeof r==`object`&&r&&!Array.isArray(r)){let e=r[t];if(typeof e==`function`)return e()}return n}var b=a([s(`descriptions`,{fontSize:`var(--n-font-size)`},[s(`descriptions-separator`,`
 display: inline-block;
 margin: 0 8px 0 2px;
 `),s(`descriptions-table-wrapper`,[s(`descriptions-table`,[s(`descriptions-table-row`,[s(`descriptions-table-header`,{padding:`var(--n-th-padding)`}),s(`descriptions-table-content`,{padding:`var(--n-td-padding)`})])])]),d(`bordered`,[s(`descriptions-table-wrapper`,[s(`descriptions-table`,[s(`descriptions-table-row`,[a(`&:last-child`,[s(`descriptions-table-content`,{paddingBottom:0})])])])])]),f(`left-label-placement`,[s(`descriptions-table-content`,[a(`> *`,{verticalAlign:`top`})])]),f(`left-label-align`,[a(`th`,{textAlign:`left`})]),f(`center-label-align`,[a(`th`,{textAlign:`center`})]),f(`right-label-align`,[a(`th`,{textAlign:`right`})]),f(`bordered`,[s(`descriptions-table-wrapper`,`
 border-radius: var(--n-border-radius);
 overflow: hidden;
 background: var(--n-merged-td-color);
 border: 1px solid var(--n-merged-border-color);
 `,[s(`descriptions-table`,[s(`descriptions-table-row`,[a(`&:not(:last-child)`,[s(`descriptions-table-content`,{borderBottom:`1px solid var(--n-merged-border-color)`}),s(`descriptions-table-header`,{borderBottom:`1px solid var(--n-merged-border-color)`})]),s(`descriptions-table-header`,`
 font-weight: 400;
 background-clip: padding-box;
 background-color: var(--n-merged-th-color);
 `,[a(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})]),s(`descriptions-table-content`,[a(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})])])])])]),s(`descriptions-header`,`
 font-weight: var(--n-th-font-weight);
 font-size: 18px;
 transition: color .3s var(--n-bezier);
 line-height: var(--n-line-height);
 margin-bottom: 16px;
 color: var(--n-title-text-color);
 `),s(`descriptions-table-wrapper`,`
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[s(`descriptions-table`,`
 width: 100%;
 border-collapse: separate;
 border-spacing: 0;
 box-sizing: border-box;
 `,[s(`descriptions-table-row`,`
 box-sizing: border-box;
 transition: border-color .3s var(--n-bezier);
 `,[s(`descriptions-table-header`,`
 font-weight: var(--n-th-font-weight);
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-th-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `),s(`descriptions-table-content`,`
 vertical-align: top;
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-td-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[o(`content`,`
 transition: color .3s var(--n-bezier);
 display: inline-block;
 color: var(--n-td-text-color);
 `)]),o(`label`,`
 font-weight: var(--n-th-font-weight);
 transition: color .3s var(--n-bezier);
 display: inline-block;
 margin-right: 14px;
 color: var(--n-th-text-color);
 `)])])])]),s(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color);
 --n-merged-td-color: var(--n-td-color);
 --n-merged-border-color: var(--n-border-color);
 `),u(s(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-modal);
 --n-merged-td-color: var(--n-td-color-modal);
 --n-merged-border-color: var(--n-border-color-modal);
 `)),i(s(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-popover);
 --n-merged-td-color: var(--n-td-color-popover);
 --n-merged-border-color: var(--n-border-color-popover);
 `))]),x=`DESCRIPTION_ITEM_FLAG`;function S(e){return typeof e==`object`&&e&&!Array.isArray(e)?e.type&&e.type.DESCRIPTION_ITEM_FLAG:!1}var C=e({name:`Descriptions`,props:Object.assign(Object.assign({},m.props),{title:String,column:{type:Number,default:3},columns:Number,labelPlacement:{type:String,default:`top`},labelAlign:{type:String,default:`left`},separator:{type:String,default:`:`},size:String,bordered:Boolean,labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]}),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:r,mergedComponentPropsRef:i}=l(e),a=n(()=>e.size||i?.value?.Descriptions?.size||`medium`),o=m(`Descriptions`,`-descriptions`,b,_,e,t),s=n(()=>{let{bordered:t}=e,n=a.value,{common:{cubicBezierEaseInOut:r},self:{titleTextColor:i,thColor:s,thColorModal:c,thColorPopover:l,thTextColor:u,thFontWeight:d,tdTextColor:f,tdColor:m,tdColorModal:h,tdColorPopover:g,borderColor:_,borderColorModal:v,borderColorPopover:y,borderRadius:b,lineHeight:x,[p(`fontSize`,n)]:S,[p(t?`thPaddingBordered`:`thPadding`,n)]:C,[p(t?`tdPaddingBordered`:`tdPadding`,n)]:w}}=o.value;return{"--n-title-text-color":i,"--n-th-padding":C,"--n-td-padding":w,"--n-font-size":S,"--n-bezier":r,"--n-th-font-weight":d,"--n-line-height":x,"--n-th-text-color":u,"--n-td-text-color":f,"--n-th-color":s,"--n-th-color-modal":c,"--n-th-color-popover":l,"--n-td-color":m,"--n-td-color-modal":h,"--n-td-color-popover":g,"--n-border-radius":b,"--n-border-color":_,"--n-border-color-modal":v,"--n-border-color-popover":y}}),u=r?c(`descriptions`,n(()=>{let t=``,{bordered:n}=e;return n&&(t+=`a`),t+=a.value[0],t}),s,e):void 0;return{mergedClsPrefix:t,cssVars:r?void 0:s,themeClass:u?.themeClass,onRender:u?.onRender,compitableColumn:h(e,[`columns`,`column`]),inlineThemeDisabled:r,mergedSize:a}},render(){let e=this.$slots.default,n=e?v(e()):[];n.length;let{contentClass:i,labelClass:a,compitableColumn:o,labelPlacement:s,labelAlign:c,mergedSize:l,bordered:u,title:d,cssVars:f,mergedClsPrefix:p,separator:m,onRender:h}=this;h?.();let _=n.filter(e=>S(e)),b=_.reduce((e,n,r)=>{let c=n.props||{},l=_.length-1===r,d=[`label`in c?c.label:y(n,`label`)],f=[y(n)],h=c.span||1,g=e.span;e.span+=h;let v=c.labelStyle||c[`label-style`]||this.labelStyle,b=c.contentStyle||c[`content-style`]||this.contentStyle;if(s===`left`)u?e.row.push(t(`th`,{class:[`${p}-descriptions-table-header`,a],colspan:1,style:v},d),t(`td`,{class:[`${p}-descriptions-table-content`,i],colspan:l?(o-g)*2+1:h*2-1,style:b},f)):e.row.push(t(`td`,{class:`${p}-descriptions-table-content`,colspan:l?(o-g)*2:h*2},t(`span`,{class:[`${p}-descriptions-table-content__label`,a],style:v},[...d,m&&t(`span`,{class:`${p}-descriptions-separator`},m)]),t(`span`,{class:[`${p}-descriptions-table-content__content`,i],style:b},f)));else{let n=l?(o-g)*2:h*2;e.row.push(t(`th`,{class:[`${p}-descriptions-table-header`,a],colspan:n,style:v},d)),e.secondRow.push(t(`td`,{class:[`${p}-descriptions-table-content`,i],colspan:n,style:b},f))}return(e.span>=o||l)&&(e.span=0,e.row.length&&(e.rows.push(e.row),e.row=[]),s!==`left`&&e.secondRow.length&&(e.rows.push(e.secondRow),e.secondRow=[])),e},{span:0,row:[],secondRow:[],rows:[]}).rows.map(e=>t(`tr`,{class:`${p}-descriptions-table-row`},e));return t(`div`,{style:f,class:[`${p}-descriptions`,this.themeClass,`${p}-descriptions--${s}-label-placement`,`${p}-descriptions--${c}-label-align`,`${p}-descriptions--${l}-size`,u&&`${p}-descriptions--bordered`]},d||this.$slots.header?t(`div`,{class:`${p}-descriptions-header`},d||g(this,`header`)):null,t(`div`,{class:`${p}-descriptions-table-wrapper`},t(`table`,{class:`${p}-descriptions-table`},t(`tbody`,null,s===`top`&&t(`tr`,{class:`${p}-descriptions-table-row`,style:{visibility:`collapse`}},r(o*2,t(`td`,null))),b))))}}),w={label:String,span:{type:Number,default:1},labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]},T=e({name:`DescriptionsItem`,[x]:!0,props:w,slots:Object,render(){return null}});export{C as n,T as t};