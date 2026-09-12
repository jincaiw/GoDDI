import{E as e,g as t,k as n}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{Fn as r,Pn as i,Ur as a,Vr as o,gn as s,mr as c,zr as l}from"./router-BIZtGF2g.js";import{n as u}from"./index-DNIszxBR.js";var d=l(`text`,`
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
`,[o(`strong`,`
 font-weight: var(--n-font-weight-strong);
 `),o(`italic`,{fontStyle:`italic`}),o(`underline`,{textDecoration:`underline`}),o(`code`,`
 line-height: 1.4;
 display: inline-block;
 font-family: var(--n-font-famliy-mono);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 box-sizing: border-box;
 padding: .05em .35em 0 .35em;
 border-radius: var(--n-code-border-radius);
 font-size: .9em;
 color: var(--n-code-text-color);
 background-color: var(--n-code-color);
 border: var(--n-code-border);
 `)]),f=e({name:`Text`,props:Object.assign(Object.assign({},s.props),{code:Boolean,type:{type:String,default:`default`},delete:Boolean,strong:Boolean,italic:Boolean,underline:Boolean,depth:[String,Number],tag:String,as:{type:String,validator:()=>!0,default:void 0}}),setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:o}=r(e),l=s(`Typography`,`-text`,d,u,e,n),f=t(()=>{let{depth:t,type:n}=e,r=n===`default`?t===void 0?`textColor`:`textColor${t}Depth`:a(`textColor`,n),{common:{fontWeightStrong:i,fontFamilyMono:o,cubicBezierEaseInOut:s},self:{codeTextColor:c,codeBorderRadius:u,codeColor:d,codeBorder:f,[r]:p}}=l.value;return{"--n-bezier":s,"--n-text-color":p,"--n-font-weight-strong":i,"--n-font-famliy-mono":o,"--n-code-border-radius":u,"--n-code-text-color":c,"--n-code-color":d,"--n-code-border":f}}),p=o?i(`text`,t(()=>`${e.type[0]}${e.depth||``}`),f,e):void 0;return{mergedClsPrefix:n,compitableTag:c(e,[`as`,`tag`]),cssVars:o?void 0:f,themeClass:p?.themeClass,onRender:p?.onRender}},render(){var e,t;let{mergedClsPrefix:r}=this;(e=this.onRender)==null||e.call(this);let i=[`${r}-text`,this.themeClass,{[`${r}-text--code`]:this.code,[`${r}-text--delete`]:this.delete,[`${r}-text--strong`]:this.strong,[`${r}-text--italic`]:this.italic,[`${r}-text--underline`]:this.underline}],a=(t=this.$slots).default?.call(t);return this.code?n(`code`,{class:i,style:this.cssVars},this.delete?n(`del`,null,a):a):this.delete?n(`del`,{class:i,style:this.cssVars},a):n(this.compitableTag||`span`,{class:i,style:this.cssVars},a)}});export{f as t};