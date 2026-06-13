import{O as e,T as t,g as n}from"./echarts-DxBJA66o.js";import{Bt as r,It as i,Lt as a,Nt as o,Pt as s,Rt as c,Vt as l,dt as u,k as d,ut as f,zt as p}from"./auth-DEMSBBAb.js";import{D as m}from"./vue-core-RsSxNVS3.js";import{t as h}from"./use-compitable-mYvAsDS6.js";import{t as g}from"./get-slot-6kXJmSMP.js";import{h as _,st as v}from"./index-lJkVLtdC.js";function y(e,t=`default`,n=[]){let{children:r}=e;if(typeof r==`object`&&r&&!Array.isArray(r)){let e=r[t];if(typeof e==`function`)return e()}return n}var b=o([s(`descriptions`,{fontSize:`var(--n-font-size)`},[s(`descriptions-separator`,`
 display: inline-block;
 margin: 0 8px 0 2px;
 `),s(`descriptions-table-wrapper`,[s(`descriptions-table`,[s(`descriptions-table-row`,[s(`descriptions-table-header`,{padding:`var(--n-th-padding)`}),s(`descriptions-table-content`,{padding:`var(--n-td-padding)`})])])]),c(`bordered`,[s(`descriptions-table-wrapper`,[s(`descriptions-table`,[s(`descriptions-table-row`,[o(`&:last-child`,[s(`descriptions-table-content`,{paddingBottom:0})])])])])]),a(`left-label-placement`,[s(`descriptions-table-content`,[o(`> *`,{verticalAlign:`top`})])]),a(`left-label-align`,[o(`th`,{textAlign:`left`})]),a(`center-label-align`,[o(`th`,{textAlign:`center`})]),a(`right-label-align`,[o(`th`,{textAlign:`right`})]),a(`bordered`,[s(`descriptions-table-wrapper`,`
 border-radius: var(--n-border-radius);
 overflow: hidden;
 background: var(--n-merged-td-color);
 border: 1px solid var(--n-merged-border-color);
 `,[s(`descriptions-table`,[s(`descriptions-table-row`,[o(`&:not(:last-child)`,[s(`descriptions-table-content`,{borderBottom:`1px solid var(--n-merged-border-color)`}),s(`descriptions-table-header`,{borderBottom:`1px solid var(--n-merged-border-color)`})]),s(`descriptions-table-header`,`
 font-weight: 400;
 background-clip: padding-box;
 background-color: var(--n-merged-th-color);
 `,[o(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})]),s(`descriptions-table-content`,[o(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})])])])])]),s(`descriptions-header`,`
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
 `,[i(`content`,`
 transition: color .3s var(--n-bezier);
 display: inline-block;
 color: var(--n-td-text-color);
 `)]),i(`label`,`
 font-weight: var(--n-th-font-weight);
 transition: color .3s var(--n-bezier);
 display: inline-block;
 margin-right: 14px;
 color: var(--n-th-text-color);
 `)])])])]),s(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color);
 --n-merged-td-color: var(--n-td-color);
 --n-merged-border-color: var(--n-border-color);
 `),r(s(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-modal);
 --n-merged-td-color: var(--n-td-color-modal);
 --n-merged-border-color: var(--n-border-color-modal);
 `)),l(s(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-popover);
 --n-merged-td-color: var(--n-td-color-popover);
 --n-merged-border-color: var(--n-border-color-popover);
 `))]),x=`DESCRIPTION_ITEM_FLAG`;function S(e){return typeof e==`object`&&e&&!Array.isArray(e)?e.type&&e.type.DESCRIPTION_ITEM_FLAG:!1}var C=t({name:`Descriptions`,props:Object.assign(Object.assign({},d.props),{title:String,column:{type:Number,default:3},columns:Number,labelPlacement:{type:String,default:`top`},labelAlign:{type:String,default:`left`},separator:{type:String,default:`:`},size:String,bordered:Boolean,labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]}),slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:r,mergedComponentPropsRef:i}=u(e),a=n(()=>e.size||i?.value?.Descriptions?.size||`medium`),o=d(`Descriptions`,`-descriptions`,b,_,e,t),s=n(()=>{let{bordered:t}=e,n=a.value,{common:{cubicBezierEaseInOut:r},self:{titleTextColor:i,thColor:s,thColorModal:c,thColorPopover:l,thTextColor:u,thFontWeight:d,tdTextColor:f,tdColor:m,tdColorModal:h,tdColorPopover:g,borderColor:_,borderColorModal:v,borderColorPopover:y,borderRadius:b,lineHeight:x,[p(`fontSize`,n)]:S,[p(t?`thPaddingBordered`:`thPadding`,n)]:C,[p(t?`tdPaddingBordered`:`tdPadding`,n)]:w}}=o.value;return{"--n-title-text-color":i,"--n-th-padding":C,"--n-td-padding":w,"--n-font-size":S,"--n-bezier":r,"--n-th-font-weight":d,"--n-line-height":x,"--n-th-text-color":u,"--n-td-text-color":f,"--n-th-color":s,"--n-th-color-modal":c,"--n-th-color-popover":l,"--n-td-color":m,"--n-td-color-modal":h,"--n-td-color-popover":g,"--n-border-radius":b,"--n-border-color":_,"--n-border-color-modal":v,"--n-border-color-popover":y}}),c=r?f(`descriptions`,n(()=>{let t=``,{bordered:n}=e;return n&&(t+=`a`),t+=a.value[0],t}),s,e):void 0;return{mergedClsPrefix:t,cssVars:r?void 0:s,themeClass:c?.themeClass,onRender:c?.onRender,compitableColumn:h(e,[`columns`,`column`]),inlineThemeDisabled:r,mergedSize:a}},render(){let t=this.$slots.default,n=t?v(t()):[];n.length;let{contentClass:r,labelClass:i,compitableColumn:a,labelPlacement:o,labelAlign:s,mergedSize:c,bordered:l,title:u,cssVars:d,mergedClsPrefix:f,separator:p,onRender:h}=this;h?.();let _=n.filter(e=>S(e)),b=_.reduce((t,n,s)=>{let c=n.props||{},u=_.length-1===s,d=[`label`in c?c.label:y(n,`label`)],m=[y(n)],h=c.span||1,g=t.span;t.span+=h;let v=c.labelStyle||c[`label-style`]||this.labelStyle,b=c.contentStyle||c[`content-style`]||this.contentStyle;if(o===`left`)l?t.row.push(e(`th`,{class:[`${f}-descriptions-table-header`,i],colspan:1,style:v},d),e(`td`,{class:[`${f}-descriptions-table-content`,r],colspan:u?(a-g)*2+1:h*2-1,style:b},m)):t.row.push(e(`td`,{class:`${f}-descriptions-table-content`,colspan:u?(a-g)*2:h*2},e(`span`,{class:[`${f}-descriptions-table-content__label`,i],style:v},[...d,p&&e(`span`,{class:`${f}-descriptions-separator`},p)]),e(`span`,{class:[`${f}-descriptions-table-content__content`,r],style:b},m)));else{let n=u?(a-g)*2:h*2;t.row.push(e(`th`,{class:[`${f}-descriptions-table-header`,i],colspan:n,style:v},d)),t.secondRow.push(e(`td`,{class:[`${f}-descriptions-table-content`,r],colspan:n,style:b},m))}return(t.span>=a||u)&&(t.span=0,t.row.length&&(t.rows.push(t.row),t.row=[]),o!==`left`&&t.secondRow.length&&(t.rows.push(t.secondRow),t.secondRow=[])),t},{span:0,row:[],secondRow:[],rows:[]}).rows.map(t=>e(`tr`,{class:`${f}-descriptions-table-row`},t));return e(`div`,{style:d,class:[`${f}-descriptions`,this.themeClass,`${f}-descriptions--${o}-label-placement`,`${f}-descriptions--${s}-label-align`,`${f}-descriptions--${c}-size`,l&&`${f}-descriptions--bordered`]},u||this.$slots.header?e(`div`,{class:`${f}-descriptions-header`},u||g(this,`header`)):null,e(`div`,{class:`${f}-descriptions-table-wrapper`},e(`table`,{class:`${f}-descriptions-table`},e(`tbody`,null,o===`top`&&e(`tr`,{class:`${f}-descriptions-table-row`,style:{visibility:`collapse`}},m(a*2,e(`td`,null))),b))))}}),w={label:String,span:{type:Number,default:1},labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]},T=t({name:`DescriptionsItem`,[x]:!0,props:w,slots:Object,render(){return null}});export{C as n,T as t};